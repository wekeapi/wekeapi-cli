package test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"wekeapi-msg-sync/internal/config"
	"wekeapi-msg-sync/internal/store"
)

// openTestStore 在临时目录创建 SQLite 库。
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), store.Options{DBPath: path})
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestContactsCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"

	if err := st.InsertSkeletons(ctx, owner, store.TypeFriend, []string{"wxid_a", "wxid_b"}); err != nil {
		t.Fatalf("InsertSkeletons 失败: %v", err)
	}
	ids, err := st.ListContactIDs(ctx, owner, store.TypeFriend)
	if err != nil || len(ids) != 2 {
		t.Fatalf("ListContactIDs = %v, err=%v", ids, err)
	}

	if err := st.UpsertContactDetail(ctx, owner, store.TypeFriend, &store.ContactDetail{
		ContactID: "wxid_a",
		Nickname:  "Alice",
		Alias:     "alice",
		Remark:    "PM",
		Labels:    "1,2",
		RawJSON:   `{"nickName":"Alice"}`,
	}); err != nil {
		t.Fatalf("UpsertContactDetail 失败: %v", err)
	}

	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: owner, Type: store.TypeFriend, Keyword: "Ali", Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("QueryContacts 失败: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("QueryContacts total=%d items=%d, want 1", page.Total, len(page.Items))
	}
	if page.Items[0].Nickname != "Alice" || page.Items[0].Labels != "1,2" {
		t.Errorf("联系人数据错误: %+v", page.Items[0])
	}

	// ModContact 只更新非空字段，不应清空已有昵称。
	if err := st.UpsertContactFromMod(ctx, owner, store.TypeFriend, &store.ContactDetail{
		ContactID: "wxid_a",
		Remark:    "Boss",
		RawJSON:   `{"remark":{"str":"Boss"}}`,
	}); err != nil {
		t.Fatalf("UpsertContactFromMod 失败: %v", err)
	}
	page, err = st.QueryContacts(ctx, store.ContactFilter{Owner: owner, Keyword: "wxid_a", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("查询更新后的联系人 total=%d err=%v", page.Total, err)
	}
	cur := page.Items[0]
	if cur.Nickname != "Alice" {
		t.Errorf("Nickname = %q, want Alice（空值不应覆盖）", cur.Nickname)
	}
	if cur.Remark != "Boss" {
		t.Errorf("Remark = %q, want Boss", cur.Remark)
	}

	contactType, ok, err := st.ContactType(ctx, owner, "wxid_a")
	if err != nil || !ok || contactType != store.TypeFriend {
		t.Errorf("ContactType = %d, ok=%v, err=%v", contactType, ok, err)
	}

	if err := st.DeleteContact(ctx, owner, "wxid_a"); err != nil {
		t.Fatalf("DeleteContact 失败: %v", err)
	}
	ids, err = st.ListContactIDs(ctx, owner, store.TypeFriend)
	if err != nil || len(ids) != 1 || ids[0] != "wxid_b" {
		t.Errorf("删除后 ListContactIDs = %v, err=%v", ids, err)
	}
}

func TestLabels(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"

	if err := st.UpsertLabels(ctx, owner, []store.LabelRecord{{LabelID: 1, LabelName: "客户"}, {LabelID: 2, LabelName: "同行"}}); err != nil {
		t.Fatalf("UpsertLabels 失败: %v", err)
	}
	if err := st.UpsertLabels(ctx, owner, []store.LabelRecord{{LabelID: 1, LabelName: "重点客户"}}); err != nil {
		t.Fatalf("UpsertLabels 改名失败: %v", err)
	}
	labels, err := st.ListLabels(ctx, owner)
	if err != nil || len(labels) != 2 {
		t.Fatalf("ListLabels = %+v, err=%v", labels, err)
	}
	if labels[0].LabelName != "重点客户" {
		t.Errorf("标签改名未生效: %+v", labels[0])
	}
	if err := st.DeleteLabels(ctx, owner, []int64{2}); err != nil {
		t.Fatalf("DeleteLabels 失败: %v", err)
	}
	ids, err := st.ListLabelIDs(ctx, owner)
	if err != nil || len(ids) != 1 || ids[0] != 1 {
		t.Errorf("DeleteLabels 后 ListLabelIDs = %v, err=%v", ids, err)
	}
}

func TestMessages(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"
	const self = "wxid_self"
	const peer = "wxid_peer"
	const room = "123@chatroom"

	m1 := &store.MessageRecord{
		NewMsgID:   "1001",
		CreateTime: 1700000000, MsgType: 1, FromWxID: peer, ToWxID: self, Content: "你好",
	}
	inserted, err := st.InsertMessage(ctx, owner, m1)
	if err != nil || !inserted {
		t.Fatalf("InsertMessage = %v, err=%v", inserted, err)
	}
	inserted, err = st.InsertMessage(ctx, owner, m1)
	if err != nil || inserted {
		t.Fatalf("重复 InsertMessage = %v, err=%v, want false", inserted, err)
	}

	m2 := &store.MessageRecord{
		NewMsgID:   "1002",
		CreateTime: 1700000001, MsgType: 1, FromWxID: "wxid_sender", ToWxID: self,
		RoomID: room, Content: "群消息",
	}
	if _, err := st.InsertMessage(ctx, owner, m2); err != nil {
		t.Fatalf("InsertMessage 群消息失败: %v", err)
	}

	// 无 newMsgId 时用内容哈希作为消息 ID 去重。
	m3 := &store.MessageRecord{CreateTime: 1700000002, FromWxID: self, ToWxID: peer, Content: "无ID"}
	if inserted, _ := st.InsertMessage(ctx, owner, m3); !inserted {
		t.Fatal("首次插入无 ID 消息失败")
	}
	if !strings.HasPrefix(m3.NewMsgID, "h:") {
		t.Errorf("new_msg_id = %q, want h: 前缀的内容哈希", m3.NewMsgID)
	}
	if inserted, _ := st.InsertMessage(ctx, owner, m3); inserted {
		t.Fatal("重复插入无 ID 消息未去重")
	}

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: owner, ConversationID: "S:" + peer, Page: 1, Limit: 10})
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("私聊消息 total=%d items=%d err=%v", page.Total, len(page.Items), err)
	}
	page, err = st.QueryMessages(ctx, store.MessageFilter{Owner: owner, ConversationID: "R:" + room, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("群消息 total=%d items=%d err=%v", page.Total, len(page.Items), err)
	}
	if page.Items[0].RoomID != room || page.Items[0].FromWxID != "wxid_sender" {
		t.Errorf("群消息字段错误: %+v", page.Items[0])
	}

	page, err = st.QueryMessages(ctx, store.MessageFilter{Owner: owner, Keyword: "你好", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("关键词过滤 total=%d err=%v", page.Total, err)
	}

	counts, err := st.Counts(ctx, owner)
	if err != nil {
		t.Fatalf("Counts 失败: %v", err)
	}
	if counts.Messages != 3 {
		t.Errorf("Messages = %d, want 3", counts.Messages)
	}
}

func TestSyncState(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"

	stt, err := st.GetSyncState(ctx, owner)
	if err != nil || stt.Status != store.SyncStatusIdle {
		t.Fatalf("默认同步状态 = %+v, err=%v", stt, err)
	}
	if err := st.SetSyncState(ctx, owner, store.SyncStatusError, "2024-01-01 00:00:00", "boom"); err != nil {
		t.Fatalf("SetSyncState 失败: %v", err)
	}
	stt, err = st.GetSyncState(ctx, owner)
	if err != nil || stt.Status != store.SyncStatusError || stt.LastError != "boom" {
		t.Fatalf("同步状态 = %+v, err=%v", stt, err)
	}
}

// TestQueryContactsByLabels 验证按标签查询联系人：单标签、多标签同时具备，
// 以及标签 ID 前缀不误匹配（1 不应命中 12）。
func TestQueryContactsByLabels(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"

	seeds := []struct {
		id     string
		labels string
	}{
		{"wxid_a", "1,2"},
		{"wxid_b", "2"},
		{"wxid_c", "12"},
	}
	for _, c := range seeds {
		if err := st.UpsertContactDetail(ctx, owner, store.TypeFriend, &store.ContactDetail{
			ContactID: c.id,
			Nickname:  c.id,
			Labels:    c.labels,
		}); err != nil {
			t.Fatalf("UpsertContactDetail(%s) 失败: %v", c.id, err)
		}
	}

	query := func(labels []int64) *store.ContactPage {
		t.Helper()
		page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: owner, Labels: labels, Page: 1, Limit: 20})
		if err != nil {
			t.Fatalf("QueryContacts(labels=%v) 失败: %v", labels, err)
		}
		if page.Total != len(page.Items) {
			t.Fatalf("labels=%v total=%d 与返回条数 %d 不一致", labels, page.Total, len(page.Items))
		}
		return page
	}

	if page := query([]int64{1}); page.Total != 1 || page.Items[0].ContactID != "wxid_a" {
		t.Errorf("标签 1 应只命中 wxid_a（不能命中标签 12），实际 total=%d", page.Total)
	}
	if page := query([]int64{2}); page.Total != 2 {
		t.Errorf("标签 2 应命中 2 个联系人，实际 %d", page.Total)
	}
	// 多个标签为「同时具备」语义。
	if page := query([]int64{1, 2}); page.Total != 1 || page.Items[0].ContactID != "wxid_a" {
		t.Errorf("同时具备标签 1、2 的应只有 wxid_a，实际 total=%d", page.Total)
	}
	if page := query([]int64{99}); page.Total != 0 {
		t.Errorf("不存在的标签不应命中任何联系人，实际 %d", page.Total)
	}
}

func TestMarkMessageRevoked(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	const owner = "app1"

	if _, err := st.InsertMessage(ctx, owner, &store.MessageRecord{
		NewMsgID:   "9001",
		CreateTime: 1700300000, MsgType: 1, FromWxID: "peer", ToWxID: "self", Content: "待撤回",
	}); err != nil {
		t.Fatalf("InsertMessage 失败: %v", err)
	}
	n, err := st.MarkMessageRevoked(ctx, owner, "9001")
	if err != nil || n != 1 {
		t.Fatalf("MarkMessageRevoked = %d, err=%v, want 1", n, err)
	}
	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: owner, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("QueryMessages total=%d err=%v", page.Total, err)
	}
	if !page.Items[0].Revoked {
		t.Error("消息未标记为已撤回")
	}
	if n, err = st.MarkMessageRevoked(ctx, owner, "9001"); err != nil || n != 0 {
		t.Errorf("重复标记 = %d, err=%v, want 0", n, err)
	}
}

// TestMySQL 仅在设置 WEKEAPI_MSG_SYNC_MYSQL_DSN 时运行，用于本地集成验证。
func TestMySQL(t *testing.T) {
	dsn := os.Getenv("WEKEAPI_MSG_SYNC_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 WEKEAPI_MSG_SYNC_MYSQL_DSN，跳过 MySQL 集成测试")
	}
	st, err := store.Open(context.Background(), store.Options{DB: &config.DBConfig{
		Driver: "mysql", DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetimeSeconds: 60,
	}})
	if err != nil {
		t.Fatalf("Open MySQL 失败: %v", err)
	}
	defer st.Close()
	if _, err := st.Counts(context.Background(), "test-owner"); err != nil {
		t.Fatalf("Counts 失败: %v", err)
	}
}

// TestMySQLAutoCreate 验证 DSN 指向的库不存在时 Open 会自动建库；
// 仅在设置 WEKEAPI_MSG_SYNC_MYSQL_DSN 时运行，测试库以 wekeapi_autotest_ 为前缀并在结束时删除。
func TestMySQLAutoCreate(t *testing.T) {
	base := os.Getenv("WEKEAPI_MSG_SYNC_MYSQL_DSN")
	if base == "" {
		t.Skip("未设置 WEKEAPI_MSG_SYNC_MYSQL_DSN，跳过 MySQL 自动建库测试")
	}
	baseCfg, err := mysql.ParseDSN(base)
	if err != nil {
		t.Fatalf("解析基准 DSN 失败: %v", err)
	}
	if baseCfg.DBName == "" {
		t.Skip("基准 DSN 未指定库名，无法派生测试库")
	}

	dbName := fmt.Sprintf("wekeapi_autotest_%d", time.Now().UnixNano())
	testCfg := *baseCfg
	testCfg.DBName = dbName

	extra := *baseCfg
	extra.DBName = ""
	server, serr := sql.Open("mysql", extra.FormatDSN())
	if serr == nil {
		defer func() {
			_, _ = server.Exec("DROP DATABASE IF EXISTS `" + dbName + "`")
			_ = server.Close()
		}()
	}

	st, err := store.Open(context.Background(), store.Options{DB: &config.DBConfig{
		Driver: "mysql", DSN: testCfg.FormatDSN(), MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetimeSeconds: 60,
	}})
	if err != nil {
		t.Fatalf("目标库不存在时应自动建库并成功打开: %v", err)
	}
	defer st.Close()
	if _, err := st.Counts(context.Background(), "test-owner"); err != nil {
		t.Fatalf("自动建库后 Counts 失败（建表可能未成功）: %v", err)
	}
}
