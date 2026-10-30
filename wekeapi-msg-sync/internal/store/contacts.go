package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// ContactDetail 联系人详情（用于写入/更新）。
type ContactDetail struct {
	ContactID   string
	Nickname    string
	Alias       string
	Avatar      string
	Remark      string
	Sex         int
	Signature   string
	Labels      string // 逗号分隔的标签 ID
	WordingID   string // 企微：企业 ID
	WordingName string // 企微：企业名称
	MemberCount int    // 群成员数（详情接口 chatroomMemberList 长度）
	RawJSON     string
}

// Contact 联系人记录。
type Contact struct {
	ID          int64
	ContactID   string
	ContactType int
	Nickname    string
	Alias       string
	Avatar      string
	Remark      string
	Sex         int
	Signature   string
	Labels      string
	WordingID   string
	WordingName string
	MemberCount int
	RawJSON     string
	CreatedAt   string
	UpdatedAt   string
}

// LabelRecord 一条微信标签。
type LabelRecord struct {
	LabelID   int64
	LabelName string
}

// ListContactIDs 返回某归属某类型的全部联系人 ID。
func (s *Store) ListContactIDs(ctx context.Context, owner string, contactType int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT contact_id FROM contacts WHERE wx_id = ? AND contact_type = ?`, normalizeOwner(owner), contactType)
	if err != nil {
		return nil, fmt.Errorf("查询本地联系人失败: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("读取联系人 ID 失败: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// InsertSkeletons 批量插入联系人骨架（仅 ID，详情待补）；已存在的忽略。
func (s *Store) InsertSkeletons(ctx context.Context, owner string, contactType int, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交成功后 rollback 无效

	query := s.insertIgnore("contacts") + `
		(wx_id, contact_id, contact_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("准备插入语句失败: %w", err)
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, normalizeOwner(owner), id, contactType, now, now); err != nil {
			return fmt.Errorf("插入联系人 %s 失败: %w", id, err)
		}
	}
	return tx.Commit()
}

// DeleteContacts 批量删除指定类型的联系人（本地有而远端没有的数据）。
func (s *Store) DeleteContacts(ctx context.Context, owner string, contactType int, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交成功后 rollback 无效

	stmt, err := tx.PrepareContext(ctx,
		`DELETE FROM contacts WHERE wx_id = ? AND contact_type = ? AND contact_id = ?`)
	if err != nil {
		return fmt.Errorf("准备删除语句失败: %w", err)
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, normalizeOwner(owner), contactType, id); err != nil {
			return fmt.Errorf("删除联系人 %s 失败: %w", id, err)
		}
	}
	return tx.Commit()
}

// DeleteContact 删除指定联系人（不限类型，供 DelContact 回调使用）。
func (s *Store) DeleteContact(ctx context.Context, owner, contactID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE wx_id = ? AND contact_id = ?`,
		normalizeOwner(owner), contactID)
	if err != nil {
		return fmt.Errorf("删除联系人 %s 失败: %w", contactID, err)
	}
	return nil
}

// ContactType 查询联系人类型；不存在时 ok=false。
func (s *Store) ContactType(ctx context.Context, owner, contactID string) (contactType int, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT contact_type FROM contacts WHERE wx_id = ? AND contact_id = ? ORDER BY id LIMIT 1`,
		normalizeOwner(owner), contactID).Scan(&contactType)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("查询联系人 %s 类型失败: %w", contactID, err)
	}
	return contactType, true, nil
}

// UpsertContactDetail 写入/更新联系人详情（同步场景，空值也覆盖，表示远端已清空）。
func (s *Store) UpsertContactDetail(ctx context.Context, owner string, contactType int, c *ContactDetail) error {
	now := nowString()
	args := []any{
		normalizeOwner(owner), c.ContactID, contactType, c.Nickname, c.Alias, c.Avatar, c.Remark, c.Sex,
		c.Signature, c.Labels, c.WordingID, c.WordingName, c.MemberCount, c.RawJSON, now, now,
	}
	var query string
	if s.dialect == MySQL {
		query = `INSERT INTO contacts
			(wx_id, contact_id, contact_type, nickname, alias, avatar, remark, sex,
			 signature, labels, wording_id, wording_name, member_count, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			  nickname = VALUES(nickname),
			  alias = VALUES(alias),
			  avatar = VALUES(avatar),
			  remark = VALUES(remark),
			  sex = VALUES(sex),
			  signature = VALUES(signature),
			  labels = VALUES(labels),
			  wording_id = VALUES(wording_id),
			  wording_name = VALUES(wording_name),
			  member_count = VALUES(member_count),
			  raw_json = VALUES(raw_json),
			  updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO contacts
			(wx_id, contact_id, contact_type, nickname, alias, avatar, remark, sex,
			 signature, labels, wording_id, wording_name, member_count, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(wx_id, contact_type, contact_id) DO UPDATE SET
			  nickname = excluded.nickname,
			  alias = excluded.alias,
			  avatar = excluded.avatar,
			  remark = excluded.remark,
			  sex = excluded.sex,
			  signature = excluded.signature,
			  labels = excluded.labels,
			  wording_id = excluded.wording_id,
			  wording_name = excluded.wording_name,
			  member_count = excluded.member_count,
			  raw_json = excluded.raw_json,
			  updated_at = excluded.updated_at`
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("写入联系人 %s 详情失败: %w", c.ContactID, err)
	}
	return nil
}

// UpsertContactFromMod 处理 ModContact 回调：只覆盖非空字段，避免缺字段的回调清空已有资料。
func (s *Store) UpsertContactFromMod(ctx context.Context, owner string, contactType int, c *ContactDetail) error {
	cur, ok, err := s.getContact(ctx, owner, c.ContactID)
	if err != nil {
		return err
	}
	if !ok {
		return s.UpsertContactDetail(ctx, owner, contactType, c)
	}

	merged := &ContactDetail{
		ContactID:   c.ContactID,
		Nickname:    cur.Nickname,
		Alias:       cur.Alias,
		Avatar:      cur.Avatar,
		Remark:      cur.Remark,
		Sex:         cur.Sex,
		Signature:   cur.Signature,
		Labels:      cur.Labels,
		WordingID:   cur.WordingID,
		WordingName: cur.WordingName,
		MemberCount: cur.MemberCount,
		RawJSON:     c.RawJSON,
	}
	mergeIfNotEmpty(&merged.Nickname, c.Nickname)
	mergeIfNotEmpty(&merged.Alias, c.Alias)
	mergeIfNotEmpty(&merged.Avatar, c.Avatar)
	mergeIfNotEmpty(&merged.Remark, c.Remark)
	mergeIfNotEmpty(&merged.Signature, c.Signature)
	mergeIfNotEmpty(&merged.Labels, c.Labels)
	mergeIfNotEmpty(&merged.WordingID, c.WordingID)
	mergeIfNotEmpty(&merged.WordingName, c.WordingName)
	if c.Sex != 0 {
		merged.Sex = c.Sex
	}
	if c.MemberCount > 0 {
		merged.MemberCount = c.MemberCount
	}
	return s.UpsertContactDetail(ctx, owner, contactType, merged)
}

func mergeIfNotEmpty(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func (s *Store) getContact(ctx context.Context, owner, contactID string) (*Contact, bool, error) {
	var c Contact
	err := s.db.QueryRowContext(ctx, `SELECT id, contact_id, contact_type,
		nickname, alias, avatar, remark, sex, signature, labels, wording_id, wording_name,
		member_count, COALESCE(raw_json, ''), created_at, updated_at
		FROM contacts WHERE wx_id = ? AND contact_id = ? ORDER BY id LIMIT 1`,
		normalizeOwner(owner), contactID).Scan(
		&c.ID, &c.ContactID, &c.ContactType, &c.Nickname, &c.Alias, &c.Avatar, &c.Remark, &c.Sex,
		&c.Signature, &c.Labels, &c.WordingID, &c.WordingName, &c.MemberCount, &c.RawJSON,
		&c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("查询联系人 %s 失败: %w", contactID, err)
	}
	return &c, true, nil
}

// ContactFilter 联系人查询条件。
type ContactFilter struct {
	Owner   string
	Type    int // 0 表示全部
	Keyword string
	Labels  []int64 // 标签 ID，多个表示「同时具备」；空表示不限
	Page    int
	Limit   int
}

// ContactPage 联系人分页结果。
type ContactPage struct {
	Items []Contact
	Total int
}

// QueryContacts 分页查询联系人。
func (s *Store) QueryContacts(ctx context.Context, f ContactFilter) (*ContactPage, error) {
	var where []string
	var args []any
	where = append(where, "wx_id = ?")
	args = append(args, normalizeOwner(f.Owner))
	if f.Type > 0 {
		where = append(where, "contact_type = ?")
		args = append(args, f.Type)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		where = append(where, "(nickname LIKE ? OR remark LIKE ? OR alias LIKE ? OR contact_id LIKE ?)")
		like := "%" + kw + "%"
		args = append(args, like, like, like, like)
	}
	for _, id := range f.Labels {
		where = append(where, s.labelContainsExpr("labels"))
		args = append(args, "%,"+strconv.FormatInt(id, 10)+",%")
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contacts WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计联系人失败: %w", err)
	}

	query := `SELECT id, contact_id, contact_type, nickname, alias, avatar, remark, sex, signature,
		labels, wording_id, wording_name, member_count, created_at, updated_at
		FROM contacts WHERE ` + cond + ` ORDER BY updated_at DESC, id ASC LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), f.Limit, (f.Page-1)*f.Limit)
	rows, err := s.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询联系人失败: %w", err)
	}
	defer rows.Close()

	items := make([]Contact, 0, f.Limit)
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.ContactID, &c.ContactType, &c.Nickname, &c.Alias, &c.Avatar,
			&c.Remark, &c.Sex, &c.Signature, &c.Labels, &c.WordingID, &c.WordingName,
			&c.MemberCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("读取联系人失败: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取联系人失败: %w", err)
	}
	return &ContactPage{Items: items, Total: total}, nil
}

// labelContainsExpr 生成「逗号分隔的标签 ID 列包含指定 ID」的匹配表达式（跨方言）。
// 列值形如 "1,2,3"，两侧补逗号后用 LIKE 匹配，避免 1 命中 12 这类误判。
func (s *Store) labelContainsExpr(column string) string {
	if s.dialect == MySQL {
		return "(CONCAT(',', " + column + ", ',') LIKE ?)"
	}
	return "(',' || " + column + " || ',' LIKE ?)"
}

// ListLabelIDs 返回某归属的全部标签 ID。
func (s *Store) ListLabelIDs(ctx context.Context, owner string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT label_id FROM labels WHERE wx_id = ?`, normalizeOwner(owner))
	if err != nil {
		return nil, fmt.Errorf("查询本地标签失败: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("读取标签 ID 失败: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListLabels 返回某归属的全部标签（按 label_id 升序）。
func (s *Store) ListLabels(ctx context.Context, owner string) ([]LabelRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT label_id, label_name FROM labels WHERE wx_id = ? ORDER BY label_id`, normalizeOwner(owner))
	if err != nil {
		return nil, fmt.Errorf("查询本地标签失败: %w", err)
	}
	defer rows.Close()

	out := make([]LabelRecord, 0)
	for rows.Next() {
		var l LabelRecord
		if err := rows.Scan(&l.LabelID, &l.LabelName); err != nil {
			return nil, fmt.Errorf("读取标签失败: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpsertLabels 批量写入/更新标签（改名以远端为准）。
func (s *Store) UpsertLabels(ctx context.Context, owner string, labels []LabelRecord) error {
	if len(labels) == 0 {
		return nil
	}
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交成功后 rollback 无效

	var query string
	if s.dialect == MySQL {
		query = `INSERT INTO labels (wx_id, label_id, label_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE label_name = VALUES(label_name), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO labels (wx_id, label_id, label_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(wx_id, label_id) DO UPDATE SET
			  label_name = excluded.label_name, updated_at = excluded.updated_at`
	}
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("准备插入语句失败: %w", err)
	}
	defer stmt.Close()
	for _, l := range labels {
		if _, err := stmt.ExecContext(ctx, normalizeOwner(owner), l.LabelID, l.LabelName, now, now); err != nil {
			return fmt.Errorf("写入标签 %d 失败: %w", l.LabelID, err)
		}
	}
	return tx.Commit()
}

// DeleteLabels 批量删除指定标签（本地有而远端没有的数据）。
func (s *Store) DeleteLabels(ctx context.Context, owner string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交成功后 rollback 无效

	stmt, err := tx.PrepareContext(ctx, `DELETE FROM labels WHERE wx_id = ? AND label_id = ?`)
	if err != nil {
		return fmt.Errorf("准备删除语句失败: %w", err)
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, normalizeOwner(owner), id); err != nil {
			return fmt.Errorf("删除标签 %d 失败: %w", id, err)
		}
	}
	return tx.Commit()
}
