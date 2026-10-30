// Package test 黑盒/集成测试：只经导出 API 验证 HTTP 接口、回调处理与存储。
//
// 这里放“从外部观察服务行为”的用例；依赖未导出函数的单元测试仍留在各 internal 包目录。
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"wekeapi-msg-sync/internal/callback"
	"wekeapi-msg-sync/internal/config"
	"wekeapi-msg-sync/internal/server"
	"wekeapi-msg-sync/internal/store"
)

// openStore 在临时目录创建 SQLite 库，返回库实例与路径。
func openStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api.db")
	st, err := store.Open(context.Background(), store.Options{DBPath: dbPath})
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, dbPath
}

// newHandler 创建回调处理器（owner=app1，本人 wxid_self）。
func newHandler(t *testing.T) (*callback.Handler, *store.Store) {
	t.Helper()
	st, _ := openStore(t)
	return callback.New(st, "app1", "wxid_self", func(string, ...any) {}), st
}

// newServer 创建 HTTP 处理器（与 main 启动的服务同构）。
func newServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, dbPath := openStore(t)
	cfg := &config.Config{
		Token: "tk", AppID: "app1", BaseURL: "http://127.0.0.1", WxID: "wxid_self",
		Port: 3006, DBPath: dbPath,
	}
	cb := callback.New(st, cfg.AppID, cfg.WxID, func(string, ...any) {})
	srv := server.New(cfg, st, cb, log.New(io.Discard, "", 0))
	return srv.Handler(), st
}

// sendCallback 发送一次回调，失败直接终止测试。
func sendCallback(t *testing.T, h *callback.Handler, payload string) {
	t.Helper()
	if _, err := h.HandlePayload(context.Background(), []byte(payload)); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}
}

// seedData 写入固定的联系人、消息与同步状态。
func seedData(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	const owner = "app1"

	if err := st.UpsertLabels(ctx, owner, []store.LabelRecord{
		{LabelID: 1, LabelName: "客户"},
		{LabelID: 2, LabelName: "同行"},
	}); err != nil {
		t.Fatalf("UpsertLabels 失败: %v", err)
	}
	contacts := []struct {
		typ int
		d   *store.ContactDetail
	}{
		{store.TypeFriend, &store.ContactDetail{
			ContactID: "wxid_alice", Nickname: "Alice", Alias: "alice", Remark: "客户A", Avatar: "https://a", Sex: 2, Labels: "1,2",
		}},
		{store.TypeChatroom, &store.ContactDetail{
			ContactID: "123@chatroom", Nickname: "项目群", Avatar: "https://r", MemberCount: 3,
		}},
		{store.TypeGh, &store.ContactDetail{
			ContactID: "gh_1", Nickname: "测试公众号",
		}},
	}
	for _, c := range contacts {
		if err := st.UpsertContactDetail(ctx, owner, c.typ, c.d); err != nil {
			t.Fatalf("UpsertContactDetail 失败: %v", err)
		}
	}

	messages := []*store.MessageRecord{
		{
			NewMsgID:   "1001",
			CreateTime: 1700000000, MsgType: 1, FromWxID: "wxid_alice", ToWxID: "wxid_self", Content: "你好",
		},
		{
			NewMsgID: "1002", RoomID: "123@chatroom",
			CreateTime: 1700000100, MsgType: 3, FromWxID: "wxid_bob", ToWxID: "wxid_self", Content: "群图片",
		},
	}
	for _, m := range messages {
		if _, err := st.InsertMessage(ctx, owner, m); err != nil {
			t.Fatalf("InsertMessage 失败: %v", err)
		}
	}

	if err := st.SetSyncState(ctx, owner, store.SyncStatusIdle, "2024-01-01 00:00:00", ""); err != nil {
		t.Fatalf("SetSyncState 失败: %v", err)
	}
}

// apiResponse 通用响应信封，便于断言 success/error/message。
type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// doRequest 发起一次 HTTP 请求并解析响应信封。
func doRequest(t *testing.T, h http.Handler, method, target, body string) (int, apiResponse) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s 响应不是合法 JSON（status=%d body=%s）: %v",
			method, target, rec.Code, rec.Body.String(), err)
	}
	return rec.Code, resp
}

// decodeData 把响应 data 解析为目标结构。
func decodeData[T any](t *testing.T, resp apiResponse) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		t.Fatalf("解析 data 失败: %v（data=%s）", err, string(resp.Data))
	}
	return v
}

type contactsData struct {
	Contacts []struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Nickname string   `json:"nickname"`
		Remark   string   `json:"remark"`
		Alias    string   `json:"alias"`
		Type     string   `json:"type"`
		Sex      int      `json:"sex"`
		LabelIDs []int64  `json:"label_ids"`
		Labels   []string `json:"labels"`
	} `json:"contacts"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type roomsData struct {
	Rooms []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Owner       string `json:"owner"`
		MemberCount int    `json:"member_count"`
	} `json:"rooms"`
	Total int `json:"total"`
}

type messagesData struct {
	Messages []struct {
		ID             int64  `json:"id"`
		NewMsgID       string `json:"new_msg_id"`
		ConversationID string `json:"conversation_id"`
		From           string `json:"from"`
		To             string `json:"to"`
		RoomID         string `json:"room_id"`
		Content        string `json:"content"`
		Timestamp      string `json:"timestamp"`
		CreateTime     int64  `json:"create_time"`
		Type           string `json:"type"`
		MsgType        int    `json:"msg_type"`
		Revoked        bool   `json:"revoked"`
	} `json:"messages"`
	Total int `json:"total"`
}

type syncStatusDataResp struct {
	AppID         string `json:"app_id"`
	WxID          string `json:"wxid"`
	Status        string `json:"status"`
	LastSyncTime  string `json:"last_sync_time"`
	LastError     string `json:"last_error"`
	ContactsCount int    `json:"contacts_count"`
	MessagesCount int    `json:"messages_count"`
	RoomsCount    int    `json:"rooms_count"`
	DBDriver      string `json:"db_driver"`
}
