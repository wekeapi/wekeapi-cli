package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 未注入 HTTP 客户端时，兜底客户端必须带超时；此前回退到 http.DefaultClient（无超时），
// 服务端挂住连接（如代理隧道未建立）会让命令无限卡死。
func TestFallbackHTTPClientHasTimeout(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1", Token: "t"}
	if c.httpClient().Timeout <= 0 {
		t.Fatalf("fallback http client timeout = %v, want > 0", c.httpClient().Timeout)
	}
}

func TestDoSuccess(t *testing.T) {
	var gotToken, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Access-Token")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Write([]byte(`{"code":0,"message":"OK","data":{"newMsgId":123}}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "tok123"}
	resp, raw, err := c.Do(context.Background(), "/v1/message/sendText", map[string]any{"appId": "app1", "toWxid": "wxid_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "tok123" {
		t.Errorf("X-Access-Token = %q, want tok123", gotToken)
	}
	if gotPath != "/v1/message/sendText" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["appId"] != "app1" || gotBody["toWxid"] != "wxid_a" {
		t.Errorf("body = %v", gotBody)
	}
	if resp.Code != 0 {
		t.Errorf("code = %d", resp.Code)
	}
	if string(resp.Data) != `{"newMsgId":123}` {
		t.Errorf("data = %s", resp.Data)
	}
	if len(raw) == 0 {
		t.Error("raw should not be empty")
	}
}

func TestDoBizError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":-30,"message":"不支持的格式","data":null}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "t"}
	_, raw, err := c.Do(context.Background(), "/v1/message/sendImage", map[string]any{})
	var bizErr *BizError
	if !errors.As(err, &bizErr) {
		t.Fatalf("want BizError, got %T: %v", err, err)
	}
	if bizErr.Code != -30 || bizErr.Message != "不支持的格式" {
		t.Errorf("bizErr = %+v", bizErr)
	}
	if len(raw) == 0 || len(bizErr.Raw) == 0 {
		t.Error("raw response should be preserved")
	}
}

func TestDoHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "t"}
	_, _, err := c.Do(context.Background(), "/v1/x", nil)
	var localErr *LocalError
	if !errors.As(err, &localErr) {
		t.Fatalf("want LocalError, got %T: %v", err, err)
	}
}

// 命中 -43（通讯录同步进行中）时等待重试，平台同步完成后成功返回。
func TestDoSRetriesOnSyncInProgress(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.Write([]byte(`{"code":-43,"message":"通讯录同步进行中","data":null}`))
			return
		}
		w.Write([]byte(`{"code":0,"message":"OK","data":{"friends":[]}}`))
	}))
	defer srv.Close()

	var waits []int
	c := &Client{
		BaseURL:           srv.URL,
		Token:             "t",
		SyncRetryInterval: time.Millisecond,
		OnSyncRetry:       func(attempt int, wait time.Duration) { waits = append(waits, attempt) },
	}
	resp, _, err := c.Do(context.Background(), "/v1/contact/fetchContactList", map[string]any{"appId": "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("code = %d, want 0", resp.Code)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if len(waits) != 2 {
		t.Errorf("retry notices = %d, want 2", len(waits))
	}
}

// -43 重试次数用尽后仍返回 BizError（不无限重试）。
func TestDoSExhaustsRetriesOnSyncInProgress(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"code":-43,"message":"通讯录同步进行中","data":null}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:           srv.URL,
		Token:             "t",
		SyncRetryAttempts: 3,
		SyncRetryInterval: time.Millisecond,
	}
	_, raw, err := c.Do(context.Background(), "/v1/contact/fetchContactList", map[string]any{"appId": "a"})
	var bizErr *BizError
	if !errors.As(err, &bizErr) {
		t.Fatalf("want BizError, got %T: %v", err, err)
	}
	if bizErr.Code != SyncInProgressCode {
		t.Errorf("code = %d, want %d", bizErr.Code, SyncInProgressCode)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if len(raw) == 0 {
		t.Error("raw response should be preserved")
	}
}

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("file-content"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	data, err := c.Download(context.Background(), srv.URL+"/f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "file-content" {
		t.Errorf("data = %q", data)
	}
}
