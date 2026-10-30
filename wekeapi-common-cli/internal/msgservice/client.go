package msgservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIClient wekeapi-msg-sync 本地 HTTP 客户端。
type APIClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewAPIClient 创建指向本机端口的客户端。
func NewAPIClient(port int) *APIClient {
	return &APIClient{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Get 发起 GET 请求，返回原始响应体。HTTP 非 200 时返回错误（含响应体摘要）。
func (c *APIClient) Get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	return c.do(req, target)
}

// Post 以 JSON 提交数据，返回原始响应体。HTTP 非 200 时返回错误（含响应体摘要）。
func (c *APIClient) Post(ctx context.Context, path string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}
	target := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, target)
}

func (c *APIClient) do(req *http.Request, target string) ([]byte, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	return body, nil
}

// Healthy 探测服务是否就绪。
func Healthy(ctx context.Context, port int) bool {
	cl := &APIClient{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		HTTP:    &http.Client{Timeout: 1500 * time.Millisecond},
	}
	body, err := cl.Get(ctx, "/healthz", nil)
	if err != nil {
		return false
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	return env.Code == 0 && strings.EqualFold(env.Data.Status, "ok")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SentMessage 本账号已发出的消息（CLI 发送成功后上报给服务落库）。
// From 不由调用方提供，服务端使用自己配置的 wxid；收发方向因此由 from_wxid 推导。
type SentMessage struct {
	NewMsgID   string `json:"new_msg_id,omitempty"`
	ToWxID     string `json:"to_wxid"`
	RoomID     string `json:"room_id,omitempty"`
	MsgType    int    `json:"msg_type"`
	CreateTime int64  `json:"create_time,omitempty"`
	Content    string `json:"content,omitempty"`
	RawJSON    string `json:"raw_json,omitempty"`
}

// RecordSent 把已发出的消息写入运行中的数据同步服务。
// 服务未配置或未运行时静默跳过（不阻断发送流程，也不影响 stdout 契约）。
func RecordSent(ctx context.Context, layout *Layout, msg SentMessage) error {
	cfg, err := layout.Config()
	if err != nil {
		return nil
	}
	if !Healthy(ctx, cfg.Port) {
		return nil
	}
	body, err := NewAPIClient(cfg.Port).Post(ctx, "/api/messages", msg)
	if err != nil {
		return err
	}
	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("解析服务响应失败: %w", err)
	}
	if !env.Success {
		msg := env.Error
		if msg == "" {
			msg = "未知错误"
		}
		return fmt.Errorf("服务返回失败: %s", msg)
	}
	return nil
}
