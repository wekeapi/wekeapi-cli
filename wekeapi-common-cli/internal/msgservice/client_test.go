package msgservice

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAPIGetAndHealthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"OK","data":{"status":"ok"}}`))
	})
	mux.HandleFunc("GET /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("keyword") != "测试" || r.URL.Query().Get("page") != "2" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"OK","data":{"total":0,"contacts":[]}}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	if !Healthy(context.Background(), port) {
		t.Fatalf("Healthy 应返回 true")
	}

	cl := NewAPIClient(port)
	q := url.Values{"keyword": {"测试"}, "page": {"2"}}
	body, err := cl.Get(context.Background(), "/api/contacts", q)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), `"total":0`) {
		t.Fatalf("响应不符: %s", body)
	}

	if _, err := cl.Get(context.Background(), "/api/contacts", url.Values{}); err == nil {
		t.Fatalf("缺少参数时服务返回 400，应报错")
	}
}

// TestRecordSent 验证发送消息上报：走 /api/messages，from 由服务端补，字段原样传递。
func TestRecordSent(t *testing.T) {
	var (
		gotPath string
		got     SentMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"ok"}}`))
		case "/api/messages":
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("解析上报请求失败: %v", err)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"inserted":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	port, err := strconv.Atoi(strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[1])
	if err != nil {
		t.Fatalf("解析测试端口失败: %v", err)
	}
	dir := t.TempDir()
	layout := &Layout{Home: dir, ConfigPath: filepath.Join(dir, ConfigFileName)}
	cfg := `{"token":"t","appId":"a","wxid":"wxid_self","port":` + strconv.Itoa(port) + `,"db_path":"x.db"}`
	if err := os.WriteFile(layout.ConfigPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("写入服务配置失败: %v", err)
	}

	msg := SentMessage{NewMsgID: "2001", ToWxID: "wxid_peer", MsgType: 1, CreateTime: 1700000200, Content: "我发的"}
	if err := RecordSent(context.Background(), layout, msg); err != nil {
		t.Fatalf("RecordSent 失败: %v", err)
	}
	if gotPath != "/api/messages" {
		t.Errorf("请求路径 = %q, want /api/messages", gotPath)
	}
	if got.NewMsgID != msg.NewMsgID || got.ToWxID != msg.ToWxID || got.MsgType != msg.MsgType ||
		got.CreateTime != msg.CreateTime || got.Content != msg.Content {
		t.Errorf("上报内容不符: %+v", got)
	}
}

// TestRecordSentSkipsWhenServiceUnavailable 未配置/未运行时静默跳过，不影响发送结果。
func TestRecordSentSkipsWhenServiceUnavailable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	layout := &Layout{Home: dir, ConfigPath: filepath.Join(dir, ConfigFileName)}

	if err := RecordSent(ctx, layout, SentMessage{ToWxID: "wxid_peer", MsgType: 1}); err != nil {
		t.Fatalf("未配置服务时应跳过，实际: %v", err)
	}
	cfg := `{"token":"t","appId":"a","wxid":"wxid_self","port":1,"db_path":"x.db"}`
	if err := os.WriteFile(layout.ConfigPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("写入服务配置失败: %v", err)
	}
	if err := RecordSent(ctx, layout, SentMessage{ToWxID: "wxid_peer", MsgType: 1}); err != nil {
		t.Fatalf("服务未运行时应跳过，实际: %v", err)
	}
}

// TestRecordSentReportsServiceError 服务返回 success=false 时应返回错误（由调用方仅打印 warn）。
func TestRecordSentReportsServiceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"ok"}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"error":"to_wxid 不能为空","code":"INVALID_PARAMS"}`))
	}))
	defer srv.Close()

	port, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[1])
	dir := t.TempDir()
	layout := &Layout{Home: dir, ConfigPath: filepath.Join(dir, ConfigFileName)}
	cfg := `{"token":"t","appId":"a","wxid":"wxid_self","port":` + strconv.Itoa(port) + `,"db_path":"x.db"}`
	if err := os.WriteFile(layout.ConfigPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("写入服务配置失败: %v", err)
	}

	err := RecordSent(context.Background(), layout, SentMessage{ToWxID: "", MsgType: 1})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("服务端错误应返回，实际: %v", err)
	}
}
