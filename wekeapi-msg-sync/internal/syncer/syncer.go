// Package syncer 负责联系人同步：服务启动（登录成功后由 CLI 拉起）时做一次全量同步，
// 之后的变更依赖消息回调（AddMsg/ModContacts/DelContact）增量更新，不再做定时同步。
package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wekeapi-msg-sync/internal/client"
	"wekeapi-msg-sync/internal/config"
	"wekeapi-msg-sync/internal/store"
)

// Logf 日志函数。
type Logf func(format string, args ...any)

const (
	detailBatchSize     = 10 // 详情接口单批数量（服务端上限 10）
	detailBatchInterval = 200 * time.Millisecond
)

// systemAccounts 系统账号，同步好友时过滤。
var systemAccounts = map[string]bool{
	"tmessage": true, "medianote": true, "qmessage": true, "qqmail": true,
	"qqsafe": true, "weixin": true, "exmail_tool": true, "filehelper": true,
	"fmessage": true, "floatbottle": true,
}

// Syncer 联系人同步器。
type Syncer struct {
	cfg  *config.Config
	cl   *client.Client
	st   *store.Store
	logf Logf
}

// New 创建同步器。
func New(cfg *config.Config, cl *client.Client, st *store.Store, logf Logf) *Syncer {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Syncer{cfg: cfg, cl: cl, st: st, logf: logf}
}

// Run 执行一次全量同步；ctx 取消后退出。
func (s *Syncer) Run(ctx context.Context) {
	s.runOnce(ctx)
}

func (s *Syncer) runOnce(ctx context.Context) {
	owner := s.cfg.AppID
	prev, err := s.st.GetSyncState(ctx, owner)
	lastSyncAt := ""
	if err == nil {
		lastSyncAt = prev.LastSyncAt
	}
	if err := s.st.SetSyncState(ctx, owner, store.SyncStatusSyncing, lastSyncAt, ""); err != nil {
		s.logf("写入同步状态失败: %v", err)
	}

	start := time.Now()
	s.logf("开始全量同步（appId=%s）", owner)

	err = s.syncAll(ctx)
	finishedAt := time.Now().UTC().Format(time.DateTime)
	if err != nil {
		msg := truncate(err.Error(), 1000)
		if serr := s.st.SetSyncState(ctx, owner, store.SyncStatusError, finishedAt, msg); serr != nil {
			s.logf("写入同步状态失败: %v", serr)
		}
		s.logf("全量同步结束，存在错误（耗时 %s）: %v", time.Since(start).Round(time.Millisecond), err)
		return
	}
	if serr := s.st.SetSyncState(ctx, owner, store.SyncStatusIdle, finishedAt, ""); serr != nil {
		s.logf("写入同步状态失败: %v", serr)
	}
	s.logf("全量同步完成（耗时 %s）", time.Since(start).Round(time.Millisecond))
}

func (s *Syncer) syncAll(ctx context.Context) error {
	var errs []string
	addErr := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if err := s.syncLabels(ctx); err != nil {
		addErr("标签同步失败: %v", err)
	}

	list, err := s.fetchContactList(ctx)
	if err != nil {
		addErr("拉取通讯录列表失败: %v", err)
	} else {
		if err := s.syncContactType(ctx, store.TypeFriend, filterSystem(list.Friends), true); err != nil {
			addErr("好友同步失败: %v", err)
		}
		if err := s.syncContactType(ctx, store.TypeChatroom, list.Chatrooms, false); err != nil {
			addErr("群聊同步失败: %v", err)
		}
		if err := s.syncContactType(ctx, store.TypeGh, list.Ghs, true); err != nil {
			addErr("公众号同步失败: %v", err)
		}
	}

	if err := s.syncWework(ctx); err != nil {
		addErr("企微好友同步失败: %v", err)
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "；"))
}

type labelInfo struct {
	LabelID   int64  `json:"labelId"`
	LabelName string `json:"labelName"`
}

// syncLabels 全量镜像标签（改名生效，删除本地多余）。
func (s *Syncer) syncLabels(ctx context.Context) error {
	resp, _, err := s.cl.Do(ctx, "/v1/label/fetchLabelList", map[string]any{"appId": s.cfg.AppID})
	if err != nil {
		return err
	}
	var items []labelInfo
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return fmt.Errorf("解析标签列表失败: %w", err)
	}

	local, err := s.st.ListLabelIDs(ctx, s.cfg.AppID)
	if err != nil {
		return err
	}
	remote := make([]int64, 0, len(items))
	records := make([]store.LabelRecord, 0, len(items))
	for _, it := range items {
		remote = append(remote, it.LabelID)
		records = append(records, store.LabelRecord{LabelID: it.LabelID, LabelName: it.LabelName})
	}
	_, del := diff(remote, local, true)

	if err := s.st.UpsertLabels(ctx, s.cfg.AppID, records); err != nil {
		return err
	}
	if err := s.st.DeleteLabels(ctx, s.cfg.AppID, del); err != nil {
		return err
	}
	s.logf("标签：远端 %d，本地 %d，删除 %d", len(remote), len(local), len(del))
	return nil
}

type contactListResp struct {
	Friends   []string `json:"friends"`
	Chatrooms []string `json:"chatrooms"`
	Ghs       []string `json:"ghs"`
}

func (s *Syncer) fetchContactList(ctx context.Context) (*contactListResp, error) {
	resp, _, err := s.cl.Do(ctx, "/v1/contact/fetchContactList", map[string]any{"appId": s.cfg.AppID})
	if err != nil {
		return nil, err
	}
	var data contactListResp
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("解析通讯录列表失败: %w", err)
	}
	return &data, nil
}

// syncContactType 同步一类联系人：差量比对 -> 骨架/删除 -> 拉取全部远端详情。
// 仅在服务启动时执行一次，后续变更依赖回调。
func (s *Syncer) syncContactType(ctx context.Context, contactType int, remote []string, allowDelete bool) error {
	owner := s.cfg.AppID
	typeName := typeNameOf(contactType)

	local, err := s.st.ListContactIDs(ctx, owner, contactType)
	if err != nil {
		return err
	}
	add, del := diff(remote, local, allowDelete)

	if err := s.st.InsertSkeletons(ctx, owner, contactType, add); err != nil {
		return err
	}
	if allowDelete && len(del) > 0 {
		if err := s.st.DeleteContacts(ctx, owner, contactType, del); err != nil {
			return err
		}
	}

	s.logf("%s：远端 %d，本地 %d，新增 %d，删除 %d",
		typeName, len(remote), len(local), len(add), len(del))
	if len(remote) == 0 {
		return nil
	}
	return s.fetchDetails(ctx, contactType, remote)
}

// fetchDetails 分批拉取详情并入库；单批失败不中断后续批次。
func (s *Syncer) fetchDetails(ctx context.Context, contactType int, wxids []string) error {
	var errs []string
	for beg := 0; beg < len(wxids); beg += detailBatchSize {
		end := beg + detailBatchSize
		if end > len(wxids) {
			end = len(wxids)
		}
		batch := wxids[beg:end]

		details, err := s.cl.FetchContactDetails(ctx, s.cfg.AppID, batch)
		if err != nil {
			errs = append(errs, fmt.Sprintf("批次 %d-%d: %v", beg+1, end, err))
			s.logf("%s：拉取详情批次失败（%d 个）: %v", typeNameOf(contactType), len(batch), err)
		} else {
			for _, d := range details {
				if err := s.st.UpsertContactDetail(ctx, s.cfg.AppID, contactType, &store.ContactDetail{
					ContactID:   d.ContactID,
					Nickname:    d.Nickname,
					Alias:       d.Alias,
					Avatar:      d.Avatar,
					Remark:      d.Remark,
					Sex:         d.Sex,
					Signature:   d.Signature,
					Labels:      d.Labels,
					MemberCount: d.MemberCount,
					RawJSON:     d.RawJSON,
				}); err != nil {
					errs = append(errs, fmt.Sprintf("写入联系人 %s 失败: %v", d.ContactID, err))
				}
			}
		}

		if end < len(wxids) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(detailBatchInterval):
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%d 处失败（首个错误：%s）", len(errs), errs[0])
}

type qwInfo struct {
	ContactID      string `json:"userName"`
	Nickname       string `json:"nickName"`
	Avatar         string `json:"headImgSmall"`
	Sex            int    `json:"sex"`
	Remark         string `json:"remark"`
	EnterpriseID   string `json:"enterpriseId"`
	EnterpriseName string `json:"enterpriseName"`
}

// syncWework 同步企微好友：列表接口自带详情，每轮全量 upsert，删除远端已移除的。
func (s *Syncer) syncWework(ctx context.Context) error {
	owner := s.cfg.AppID
	resp, _, err := s.cl.Do(ctx, "/v1/contact/fetchQWContactList", map[string]any{"appId": owner})
	if err != nil {
		return err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return fmt.Errorf("解析企微好友列表失败: %w", err)
	}

	remote := make([]string, 0, len(items))
	details := make(map[string]*store.ContactDetail, len(items))
	for _, raw := range items {
		var q qwInfo
		if err := json.Unmarshal(raw, &q); err != nil {
			s.logf("解析企微好友失败: %v", err)
			continue
		}
		if q.ContactID == "" {
			continue
		}
		remote = append(remote, q.ContactID)
		details[q.ContactID] = &store.ContactDetail{
			ContactID:   q.ContactID,
			Nickname:    q.Nickname,
			Avatar:      q.Avatar,
			Remark:      q.Remark,
			Sex:         q.Sex,
			WordingID:   q.EnterpriseID,
			WordingName: q.EnterpriseName,
			RawJSON:     string(raw),
		}
	}

	local, err := s.st.ListContactIDs(ctx, owner, store.TypeWework)
	if err != nil {
		return err
	}
	add, del := diff(remote, local, true)
	if err := s.st.InsertSkeletons(ctx, owner, store.TypeWework, add); err != nil {
		return err
	}
	if err := s.st.DeleteContacts(ctx, owner, store.TypeWework, del); err != nil {
		return err
	}

	for _, id := range remote {
		if err := s.st.UpsertContactDetail(ctx, owner, store.TypeWework, details[id]); err != nil {
			return err
		}
	}
	s.logf("企微好友：远端 %d，本地 %d，新增 %d，删除 %d", len(remote), len(local), len(add), len(del))
	return nil
}

// diff 对比远端与本地 ID 列表：add = remote-local；del = local-remote。
func diff[T comparable](remote, local []T, allowDelete bool) (add, del []T) {
	remoteSet := make(map[T]bool, len(remote))
	for _, id := range remote {
		remoteSet[id] = true
	}
	localSet := make(map[T]bool, len(local))
	for _, id := range local {
		if !remoteSet[id] {
			del = append(del, id)
		}
		localSet[id] = true
	}
	for _, id := range remote {
		if !localSet[id] {
			add = append(add, id)
		}
	}
	if !allowDelete {
		del = nil
	}
	return add, del
}

// filterSystem 过滤系统账号。
func filterSystem(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !systemAccounts[id] {
			out = append(out, id)
		}
	}
	return out
}

func typeNameOf(contactType int) string {
	switch contactType {
	case store.TypeFriend:
		return "好友"
	case store.TypeChatroom:
		return "群聊"
	case store.TypeGh:
		return "公众号"
	case store.TypeWework:
		return "企微好友"
	}
	return "未知"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
