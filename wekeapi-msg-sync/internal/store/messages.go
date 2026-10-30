package store

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

// MessageRecord 一条待写入的消息（发送/接收共用）。
type MessageRecord struct {
	NewMsgID   string
	RoomID     string
	CreateTime int64
	MsgType    int
	FromWxID   string
	ToWxID     string
	Content    string
	RawJSON    string
}

// Message 查询返回的消息记录。
type Message struct {
	ID         int64
	NewMsgID   string
	RoomID     string
	CreateTime int64
	MsgType    int
	FromWxID   string
	ToWxID     string
	Content    string
	Revoked    bool
}

// InsertMessage 写入一条消息；按 (wx_id, new_msg_id) 幂等去重。
// new_msg_id 缺失时（回调/发送响应没有 newMsgId）用内容哈希兜底。
// 返回是否实际插入（重复返回 false）。
func (s *Store) InsertMessage(ctx context.Context, owner string, m *MessageRecord) (bool, error) {
	if strings.TrimSpace(m.NewMsgID) == "" {
		m.NewMsgID = fallbackMsgID(m)
	}
	now := nowString()
	query := s.insertIgnore("messages") + `
		(wx_id, new_msg_id, room_id, create_time, msg_type,
		 from_wxid, to_wxid, content, raw_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, query,
		normalizeOwner(owner), m.NewMsgID, m.RoomID, m.CreateTime, m.MsgType,
		m.FromWxID, m.ToWxID, m.Content, m.RawJSON, now, now)
	if err != nil {
		return false, fmt.Errorf("写入消息记录失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取写入结果失败: %w", err)
	}
	return n > 0, nil
}

// fallbackMsgID 用内容哈希充当消息 ID，保证无 newMsgId 时也能去重。
func fallbackMsgID(m *MessageRecord) string {
	raw := fmt.Sprintf("%d|%s|%s|%d|%s", m.CreateTime, m.FromWxID, m.ToWxID, m.MsgType, m.Content)
	sum := sha1.Sum([]byte(raw))
	return "h:" + hex.EncodeToString(sum[:])
}

// MessageFilter 消息查询条件。
type MessageFilter struct {
	Owner          string
	ConversationID string // S:<wxid> 或 R:<roomid>；空表示不筛选
	Keyword        string
	StartTime      int64 // Unix 秒，0 表示不限
	EndTime        int64
	Page           int
	Limit          int
}

// MessagePage 消息分页结果。
type MessagePage struct {
	Items []Message
	Total int
}

// QueryMessages 分页查询消息（按时间倒序）。
func (s *Store) QueryMessages(ctx context.Context, f MessageFilter) (*MessagePage, error) {
	var where []string
	var args []any
	where = append(where, "wx_id = ?")
	args = append(args, normalizeOwner(f.Owner))

	if conv := strings.TrimSpace(f.ConversationID); conv != "" {
		switch {
		case strings.HasPrefix(conv, "R:"):
			where = append(where, "room_id = ?")
			args = append(args, strings.TrimPrefix(conv, "R:"))
		case strings.HasPrefix(conv, "S:"):
			peer := strings.TrimPrefix(conv, "S:")
			where = append(where, "room_id = '' AND (from_wxid = ? OR to_wxid = ?)")
			args = append(args, peer, peer)
		default:
			return nil, fmt.Errorf("conversation_id 格式错误：私聊 S:<wxid>，群聊 R:<roomid>")
		}
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		where = append(where, "content LIKE ?")
		args = append(args, "%"+kw+"%")
	}
	if f.StartTime > 0 {
		where = append(where, "create_time >= ?")
		args = append(args, f.StartTime)
	}
	if f.EndTime > 0 {
		where = append(where, "create_time <= ?")
		args = append(args, f.EndTime)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计消息失败: %w", err)
	}

	query := `SELECT id, new_msg_id, room_id, create_time, msg_type, from_wxid, to_wxid,
		COALESCE(content, ''), revoked
		FROM messages WHERE ` + cond + ` ORDER BY create_time DESC, id DESC LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), f.Limit, (f.Page-1)*f.Limit)
	rows, err := s.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0, f.Limit)
	for rows.Next() {
		var (
			m       Message
			revoked int
		)
		if err := rows.Scan(&m.ID, &m.NewMsgID, &m.RoomID, &m.CreateTime, &m.MsgType,
			&m.FromWxID, &m.ToWxID, &m.Content, &revoked); err != nil {
			return nil, fmt.Errorf("读取消息失败: %w", err)
		}
		m.Revoked = revoked != 0
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取消息失败: %w", err)
	}
	return &MessagePage{Items: items, Total: total}, nil
}

// MarkMessageRevoked 按 newMsgId 标记消息为已撤回，返回受影响行数。
func (s *Store) MarkMessageRevoked(ctx context.Context, owner, newMsgID string) (int64, error) {
	id := strings.TrimSpace(newMsgID)
	if id == "" {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE messages SET revoked = 1, updated_at = ? WHERE wx_id = ? AND new_msg_id = ? AND revoked = 0`,
		nowString(), normalizeOwner(owner), id)
	if err != nil {
		return 0, fmt.Errorf("标记消息 %s 撤回失败: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取撤回标记结果失败: %w", err)
	}
	return n, nil
}

// Counts 数据统计。
type Counts struct {
	Contacts int
	Messages int
	Rooms    int
}

// Counts 返回某归属的联系人（不含群）、群聊与消息数量。
func (s *Store) Counts(ctx context.Context, owner string) (*Counts, error) {
	var c Counts
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN contact_type <> ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN contact_type = ? THEN 1 ELSE 0 END), 0)
		FROM contacts WHERE wx_id = ?`,
		TypeChatroom, TypeChatroom, normalizeOwner(owner)).Scan(&c.Contacts, &c.Rooms)
	if err != nil {
		return nil, fmt.Errorf("统计联系人失败: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE wx_id = ?`,
		normalizeOwner(owner)).Scan(&c.Messages); err != nil {
		return nil, fmt.Errorf("统计消息失败: %w", err)
	}
	return &c, nil
}
