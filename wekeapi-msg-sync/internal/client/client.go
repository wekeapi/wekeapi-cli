// Package client 实现 wekeapi 的 HTTP 客户端：POST JSON、X-Access-Token 鉴权、统一响应信封解析。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SyncInProgressCode 通讯录同步进行中的业务错误码：平台登录后会异步同步一次微信通讯录，
// 同步期间读取通讯录缓存的接口会返回该码，需要等待后重试。
const SyncInProgressCode = -43

// 命中 SyncInProgressCode 时的默认重试策略。
// 通讯录人数多的账号，平台首次同步耗时可长达数分钟，重试次数给足：
// 30 次 × 15s ≈ 7.5 分钟，避免全量同步刚开始就被判失败。
const (
	defaultSyncRetryAttempts = 30
	defaultSyncRetryInterval = 15 * time.Second
	defaultHTTPTimeout       = 60 * time.Second
)

// Response 对应 API 统一响应信封 {"code":0,"message":"OK","data":...}。
type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// BizError 表示 API 业务错误（HTTP 成功但 code != 0）。
type BizError struct {
	Code    int
	Message string
	Raw     []byte
}

func (e *BizError) Error() string {
	return fmt.Sprintf("api error: code=%d message=%s", e.Code, e.Message)
}

// LocalError 表示本地错误（网络、协议、序列化等）。
type LocalError struct{ Err error }

func (e *LocalError) Error() string { return e.Err.Error() }
func (e *LocalError) Unwrap() error { return e.Err }

// Client wekeapi HTTP 客户端。
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client

	// SyncRetryAttempts 命中 SyncInProgressCode 时的最多尝试次数（含首次请求）；<=0 用默认值。
	SyncRetryAttempts int
	// SyncRetryInterval 命中 SyncInProgressCode 时每次重试前的等待间隔；<=0 用默认值。
	SyncRetryInterval time.Duration
	// OnSyncRetry 命中 SyncInProgressCode 且将要等待重试时回调（可选）。
	OnSyncRetry func(attempt int, wait time.Duration)
}

// New 创建客户端。
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: defaultHTTPTimeout},
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

// Do 调用 POST {BaseURL}{path}，body 为可 JSON 序列化的对象。
// code != 0 返回 *BizError，网络/协议错误返回 *LocalError。
// 命中 SyncInProgressCode（通讯录同步进行中）时按间隔等待重试。
func (c *Client) Do(ctx context.Context, path string, body any) (*Response, []byte, error) {
	attempts := c.SyncRetryAttempts
	if attempts <= 0 {
		attempts = defaultSyncRetryAttempts
	}
	interval := c.SyncRetryInterval
	if interval <= 0 {
		interval = defaultSyncRetryInterval
	}

	var resp *Response
	var raw []byte
	var err error
	for attempt := 1; ; attempt++ {
		resp, raw, err = c.do(ctx, path, body)
		if err == nil {
			return resp, raw, nil
		}
		var bizErr *BizError
		if !errors.As(err, &bizErr) || bizErr.Code != SyncInProgressCode || attempt >= attempts {
			return resp, raw, err
		}
		if c.OnSyncRetry != nil {
			c.OnSyncRetry(attempt, interval)
		}
		select {
		case <-ctx.Done():
			return resp, raw, err
		case <-time.After(interval):
		}
	}
}

func (c *Client) do(ctx context.Context, path string, body any) (*Response, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, &LocalError{Err: fmt.Errorf("marshal body: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, &LocalError{Err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("X-Access-Token", c.Token)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, nil, &LocalError{Err: fmt.Errorf("request %s: %w", path, err)}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &LocalError{Err: fmt.Errorf("read response: %w", err)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, raw, &LocalError{Err: fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(raw), 512))}
	}

	var envelope Response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, raw, &LocalError{Err: fmt.Errorf("parse response: %w (body: %s)", err, truncate(string(raw), 512))}
	}
	if envelope.Code != 0 {
		return &envelope, raw, &BizError{Code: envelope.Code, Message: envelope.Message, Raw: raw}
	}
	return &envelope, raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ContactDetail getDetailInfo 返回的联系人详情。
type ContactDetail struct {
	ContactID   string `json:"wxid"`
	Nickname    string `json:"nickName"`
	Alias       string `json:"alias"`
	Avatar      string `json:"headImgSmall"`
	Sex         int    `json:"sex"`
	Remark      string `json:"remark"`
	Signature   string `json:"signature"`
	Labels      string `json:"labelList"`
	MemberCount int    `json:"-"`
	RawJSON     string `json:"-"`
}

// FetchContactDetails 调用 getDetailInfo 拉取联系人详情（单批最多 10 个）。
func (c *Client) FetchContactDetails(ctx context.Context, appID string, wxids []string) ([]ContactDetail, error) {
	if len(wxids) == 0 {
		return nil, nil
	}
	resp, _, err := c.Do(ctx, "/v1/contact/getDetailInfo", map[string]any{
		"appId": appID,
		"wxids": wxids,
	})
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return nil, fmt.Errorf("解析联系人详情失败: %w", err)
	}
	out := make([]ContactDetail, 0, len(items))
	for _, raw := range items {
		var d ContactDetail
		if err := json.Unmarshal(raw, &d); err != nil || d.ContactID == "" {
			continue
		}
		var probe struct {
			Members []json.RawMessage `json:"chatroomMemberList"`
		}
		_ = json.Unmarshal(raw, &probe)
		d.MemberCount = len(probe.Members)
		d.RawJSON = string(raw)
		out = append(out, d)
	}
	return out, nil
}
