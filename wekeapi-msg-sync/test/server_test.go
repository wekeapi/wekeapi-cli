package test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"wekeapi-msg-sync/internal/server"
	"wekeapi-msg-sync/internal/store"
)

func TestHealthEndpoint(t *testing.T) {
	h, _ := newServer(t)
	code, resp := doRequest(t, h, http.MethodGet, "/healthz", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("healthz code=%d resp=%+v", code, resp)
	}
	data := decodeData[map[string]any](t, resp)
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
}

func TestContactsEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)

	code, resp := doRequest(t, h, http.MethodGet, "/api/contacts", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("contacts code=%d resp=%+v", code, resp)
	}
	all := decodeData[contactsData](t, resp)
	if all.Total != 3 || len(all.Contacts) != 3 {
		t.Fatalf("contacts total=%d len=%d, want 3", all.Total, len(all.Contacts))
	}

	code, resp = doRequest(t, h, http.MethodGet, "/api/contacts?type=friend", "")
	if code != http.StatusOK {
		t.Fatalf("type=friend code=%d resp=%+v", code, resp)
	}
	friends := decodeData[contactsData](t, resp)
	if friends.Total != 1 || len(friends.Contacts) != 1 {
		t.Fatalf("friend total=%d len=%d, want 1", friends.Total, len(friends.Contacts))
	}
	f := friends.Contacts[0]
	if f.ID != "wxid_alice" || f.Type != "friend" || f.Name != "Alice" || f.Sex != 2 {
		t.Errorf("好友字段错误: %+v", f)
	}
	if len(f.LabelIDs) != 2 || f.LabelIDs[0] != 1 || f.LabelIDs[1] != 2 {
		t.Errorf("label_ids = %v, want [1 2]", f.LabelIDs)
	}
	if len(f.Labels) != 2 || f.Labels[0] != "客户" || f.Labels[1] != "同行" {
		t.Errorf("labels = %v, want [客户 同行]", f.Labels)
	}

	// 关键词匹配昵称。
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?keyword=项目", "")
	rooms := decodeData[contactsData](t, resp)
	if rooms.Total != 1 || rooms.Contacts[0].ID != "123@chatroom" {
		t.Errorf("关键词查询结果错误: %+v", rooms)
	}

	// 分页：total 不变，第二页只剩 1 条。
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?page=2&limit=2", "")
	paged := decodeData[contactsData](t, resp)
	if paged.Total != 3 || paged.Page != 2 || paged.Limit != 2 || len(paged.Contacts) != 1 {
		t.Errorf("分页结果错误: %+v", paged)
	}

	// 显式传本服务 app_id 允许。
	if code, resp = doRequest(t, h, http.MethodGet, "/api/contacts?app_id=app1", ""); code != http.StatusOK {
		t.Errorf("app_id=app1 code=%d resp=%+v", code, resp)
	}

	badCases := []string{
		"/api/contacts?app_id=other",
		"/api/contacts?type=unknown",
		"/api/contacts?page=0",
		"/api/contacts?limit=101",
	}
	for _, target := range badCases {
		code, resp = doRequest(t, h, http.MethodGet, target, "")
		if code != http.StatusBadRequest || resp.Success || resp.Code != server.CodeInvalidParams {
			t.Errorf("%s code=%d resp=%+v, want 400 INVALID_PARAMS", target, code, resp)
		}
	}
}

// TestContactsLabelEndpoint 验证按标签查找联系人的接口：标签名称/ID、多标签同时具备、
// 重复传参与逗号分隔等价，以及标签名不存在时返回 400。
func TestContactsLabelEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)

	// 名称过滤：wxid_alice 带「客户」「同行」两个标签。
	code, resp := doRequest(t, h, http.MethodGet, "/api/contacts?label=客户", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("label=客户 code=%d resp=%+v", code, resp)
	}
	data := decodeData[contactsData](t, resp)
	if data.Total != 1 || data.Contacts[0].ID != "wxid_alice" {
		t.Errorf("label=客户 结果错误: %+v", data)
	}

	// 多个标签为「同时具备」语义。
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?label=客户,同行", "")
	if got := decodeData[contactsData](t, resp); got.Total != 1 || got.Contacts[0].ID != "wxid_alice" {
		t.Errorf("label=客户,同行 total=%d, want 1", got.Total)
	}

	// 重复传参与逗号分隔等价。
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?label=客户&label=同行", "")
	if got := decodeData[contactsData](t, resp); got.Total != 1 {
		t.Errorf("重复 label 参数 total=%d, want 1", got.Total)
	}

	// 纯数字按标签 ID 处理；不存在的 ID 返回空结果。
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?label=1", "")
	if got := decodeData[contactsData](t, resp); got.Total != 1 || got.Contacts[0].ID != "wxid_alice" {
		t.Errorf("label=1 total=%d, want 1", got.Total)
	}
	_, resp = doRequest(t, h, http.MethodGet, "/api/contacts?label=99", "")
	if got := decodeData[contactsData](t, resp); got.Total != 0 {
		t.Errorf("label=99 total=%d, want 0", got.Total)
	}

	// 标签名称不存在 → 400。
	code, resp = doRequest(t, h, http.MethodGet, "/api/contacts?label=不存在的标签", "")
	if code != http.StatusBadRequest || resp.Success || resp.Code != server.CodeInvalidParams {
		t.Errorf("不存在的标签 code=%d resp=%+v, want 400 INVALID_PARAMS", code, resp)
	}
}

func TestRoomsEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)

	code, resp := doRequest(t, h, http.MethodGet, "/api/rooms", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("rooms code=%d resp=%+v", code, resp)
	}
	data := decodeData[roomsData](t, resp)
	if data.Total != 1 || len(data.Rooms) != 1 {
		t.Fatalf("rooms total=%d len=%d, want 1", data.Total, len(data.Rooms))
	}
	r := data.Rooms[0]
	if r.ID != "123@chatroom" || r.Name != "项目群" || r.MemberCount != 3 || r.Owner != "" {
		t.Errorf("群聊字段错误: %+v", r)
	}

	_, resp = doRequest(t, h, http.MethodGet, "/api/rooms?keyword=不存在", "")
	if rooms := decodeData[roomsData](t, resp); rooms.Total != 0 {
		t.Errorf("无匹配群聊 total=%d, want 0", rooms.Total)
	}
}

func TestMessagesEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)
	if _, err := st.MarkMessageRevoked(context.Background(), "app1", "1001"); err != nil {
		t.Fatalf("标记撤回失败: %v", err)
	}

	code, resp := doRequest(t, h, http.MethodGet, "/api/messages", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("messages code=%d resp=%+v", code, resp)
	}
	all := decodeData[messagesData](t, resp)
	if all.Total != 2 || len(all.Messages) != 2 {
		t.Fatalf("messages total=%d len=%d, want 2", all.Total, len(all.Messages))
	}
	// 按时间倒序：群图片（1002）在前。
	if all.Messages[0].NewMsgID != "1002" || all.Messages[0].Type != "image" {
		t.Errorf("消息倒序或类型映射错误: %+v", all.Messages[0])
	}

	_, resp = doRequest(t, h, http.MethodGet, "/api/messages?conversation_id=S:wxid_alice", "")
	direct := decodeData[messagesData](t, resp)
	if direct.Total != 1 {
		t.Fatalf("私聊 total=%d, want 1", direct.Total)
	}
	m := direct.Messages[0]
	if m.ConversationID != "S:wxid_alice" || m.From != "wxid_alice" || m.To != "wxid_self" ||
		m.Content != "你好" || m.Type != "text" {
		t.Errorf("私聊消息字段错误: %+v", m)
	}
	if !m.Revoked {
		t.Errorf("撤回标记未透出: %+v", m)
	}
	if m.Timestamp != "2023-11-14T22:13:20Z" {
		t.Errorf("timestamp = %q, want 2023-11-14T22:13:20Z", m.Timestamp)
	}

	_, resp = doRequest(t, h, http.MethodGet, "/api/messages?conversation_id=R:123@chatroom", "")
	group := decodeData[messagesData](t, resp)
	if group.Total != 1 {
		t.Fatalf("群消息 total=%d, want 1", group.Total)
	}
	if g := group.Messages[0]; g.RoomID != "123@chatroom" || g.ConversationID != "R:123@chatroom" || g.From != "wxid_bob" {
		t.Errorf("群消息字段错误: %+v", g)
	}

	// 时间过滤。
	_, resp = doRequest(t, h, http.MethodGet, "/api/messages?start_time=1700000100&end_time=1700000100", "")
	if filtered := decodeData[messagesData](t, resp); filtered.Total != 1 || filtered.Messages[0].NewMsgID != "1002" {
		t.Errorf("时间过滤结果错误: %+v", filtered)
	}

	badCases := []string{
		"/api/messages?conversation_id=abc",
		"/api/messages?conversation_id=S:",
		"/api/messages?start_time=abc",
		"/api/messages?start_time=1700000200&end_time=1700000100",
		"/api/messages?page=0",
	}
	for _, target := range badCases {
		code, resp = doRequest(t, h, http.MethodGet, target, "")
		if code != http.StatusBadRequest || resp.Code != server.CodeInvalidParams {
			t.Errorf("%s code=%d resp=%+v, want 400 INVALID_PARAMS", target, code, resp)
		}
	}
}

func TestCreateMessageEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)

	// 私聊：from_wxid 取服务配置的 wxid，会话按对端推导。
	code, resp := doRequest(t, h, http.MethodPost, "/api/messages",
		`{"new_msg_id":"2001","to_wxid":"wxid_peer","msg_type":1,"create_time":1700000200,"content":"我发的"}`)
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("记录消息 code=%d resp=%+v", code, resp)
	}
	created := decodeData[map[string]any](t, resp)
	if created["inserted"] != true || created["new_msg_id"] != "2001" {
		t.Fatalf("记录结果不符: %+v", created)
	}

	// 同一 newMsgId 重复上报不重复入库。
	_, resp = doRequest(t, h, http.MethodPost, "/api/messages",
		`{"new_msg_id":"2001","to_wxid":"wxid_peer","msg_type":1,"create_time":1700000200,"content":"我发的"}`)
	if again := decodeData[map[string]any](t, resp); again["inserted"] != false {
		t.Errorf("重复上报应去重: %+v", again)
	}

	_, resp = doRequest(t, h, http.MethodGet, "/api/messages?conversation_id=S:wxid_peer", "")
	direct := decodeData[messagesData](t, resp)
	if direct.Total != 1 {
		t.Fatalf("发出的私聊消息 total=%d, want 1", direct.Total)
	}
	if m := direct.Messages[0]; m.From != "wxid_self" || m.To != "wxid_peer" ||
		m.ConversationID != "S:wxid_peer" || m.Content != "我发的" {
		t.Errorf("发出的私聊消息字段错误: %+v", m)
	}

	// 群消息：room_id 由 to_wxid 推导。
	if code, resp = doRequest(t, h, http.MethodPost, "/api/messages",
		`{"new_msg_id":"2002","to_wxid":"456@chatroom","msg_type":49,"content":"群里的文件"}`); code != http.StatusOK {
		t.Fatalf("记录群消息 code=%d resp=%+v", code, resp)
	}
	_, resp = doRequest(t, h, http.MethodGet, "/api/messages?conversation_id=R:456@chatroom", "")
	group := decodeData[messagesData](t, resp)
	if group.Total != 1 || group.Messages[0].ConversationID != "R:456@chatroom" || group.Messages[0].From != "wxid_self" {
		t.Errorf("发出的群消息错误: %+v", group)
	}

	badCases := []string{
		`{"to_wxid":"","msg_type":1}`,
		`{"to_wxid":"wxid_peer","msg_type":0}`,
		`not-json`,
	}
	for _, b := range badCases {
		code, resp = doRequest(t, h, http.MethodPost, "/api/messages", b)
		if code != http.StatusBadRequest || resp.Code != server.CodeInvalidParams {
			t.Errorf("body=%s code=%d resp=%+v, want 400 INVALID_PARAMS", b, code, resp)
		}
	}
}

func TestSyncStatusEndpoint(t *testing.T) {
	h, st := newServer(t)
	seedData(t, st)

	code, resp := doRequest(t, h, http.MethodGet, "/api/sync-status", "")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("sync-status code=%d resp=%+v", code, resp)
	}
	data := decodeData[syncStatusDataResp](t, resp)
	if data.AppID != "app1" || data.WxID != "wxid_self" || data.Status != store.SyncStatusIdle {
		t.Errorf("同步状态字段错误: %+v", data)
	}
	if data.ContactsCount != 2 || data.MessagesCount != 2 || data.RoomsCount != 1 {
		t.Errorf("统计数字错误: %+v", data)
	}
	if data.DBDriver != "sqlite" {
		t.Errorf("db_driver = %q, want sqlite", data.DBDriver)
	}
	if data.LastSyncTime != "2024-01-01T00:00:00Z" {
		t.Errorf("last_sync_time = %q, want 2024-01-01T00:00:00Z", data.LastSyncTime)
	}

	code, resp = doRequest(t, h, http.MethodGet, "/api/sync-status?app_id=other", "")
	if code != http.StatusBadRequest || resp.Code != server.CodeInvalidParams {
		t.Errorf("app_id 不一致 code=%d resp=%+v, want 400", code, resp)
	}
}

func TestCallbackEndpoint(t *testing.T) {
	h, _ := newServer(t)

	payload := `{"appId":"app1","wxid":"wxid_self","msgFlag":"AddMsg","msgData":{
		"msg_type":1,
		"from_user_name":{"str":"wxid_peer"},
		"to_user_name":{"str":"wxid_self"},
		"content":{"str":"回调测试"},
		"new_msg_id":2001,
		"create_time":1700000200
	}}`
	code, resp := doRequest(t, h, http.MethodPost, "/callback", payload)
	if code != http.StatusOK || !resp.Success || !strings.Contains(resp.Message, "processed") {
		t.Fatalf("callback code=%d resp=%+v", code, resp)
	}

	_, resp = doRequest(t, h, http.MethodGet, "/api/messages", "")
	messages := decodeData[messagesData](t, resp)
	if messages.Total != 1 {
		t.Fatalf("回调后消息 total=%d, want 1", messages.Total)
	}
	if m := messages.Messages[0]; m.NewMsgID != "2001" || m.Content != "回调测试" || m.ConversationID != "S:wxid_peer" {
		t.Errorf("回调入库消息字段错误: %+v", m)
	}

	// 重复回调只入库一次。
	if code, resp = doRequest(t, h, http.MethodPost, "/callback", payload); code != http.StatusOK || !resp.Success {
		t.Fatalf("重复回调 code=%d resp=%+v", code, resp)
	}
	_, resp = doRequest(t, h, http.MethodGet, "/api/messages", "")
	if messages = decodeData[messagesData](t, resp); messages.Total != 1 {
		t.Errorf("重复回调未去重 total=%d", messages.Total)
	}

	// appId 不匹配：200 忽略而非报错。
	ignored := `{"appId":"other","msgFlag":"AddMsg","msgData":{}}`
	if code, resp = doRequest(t, h, http.MethodPost, "/callback", ignored); code != http.StatusOK || !strings.Contains(resp.Message, "mismatch") {
		t.Errorf("appId 不匹配 code=%d resp=%+v", code, resp)
	}

	// 非法请求体：400。
	if code, resp = doRequest(t, h, http.MethodPost, "/callback", "not-json"); code != http.StatusBadRequest || resp.Code != server.CodeInvalidParams {
		t.Errorf("非法回调 code=%d resp=%+v, want 400", code, resp)
	}
}
