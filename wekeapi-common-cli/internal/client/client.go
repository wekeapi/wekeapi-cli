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
	"time"
)

// SyncInProgressCode 通讯录同步进行中的业务错误码：账号登录后平台会异步同步一次微信通讯录，
// 同步期间读取通讯录缓存的接口（fetchContactList/getBriefInfo/getDetailInfo 等）返回该码，
// 需等待平台同步完成后重试。
const SyncInProgressCode = -43

// 命中 SyncInProgressCode 时的默认等待重试策略（平台同步耗时较长，一遇 -43 立即失败会让上层整体失败）。
const (
	defaultSyncRetryAttempts = 5                // 最多尝试次数（含首次请求）
	defaultSyncRetryInterval = 15 * time.Second // 每次重试前的等待间隔
)

// Response 对应 API 统一响应信封 {"code":0,"message":"OK","data":...}。
type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// BizError 表示 API 业务错误（HTTP 成功但 code != 0），对应退出码 1。
type BizError struct {
	Code    int
	Message string
	Raw     []byte // 原始响应体，供透传输出
}

func (e *BizError) Error() string {
	return fmt.Sprintf("api error: code=%d message=%s", e.Code, e.Message)
}

// LocalError 表示本地错误（参数、网络、文件读写等），对应退出码 2。
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
	// OnSyncRetry 命中 SyncInProgressCode 且将要等待重试时回调（可选），供调用方输出等待提示。
	OnSyncRetry func(attempt int, wait time.Duration)
}

// defaultHTTPTimeout 未显式注入 HTTP 客户端时的兜底超时，与 CLI 全局 --timeout 默认值一致。
// 不得退化为 http.DefaultClient（无超时）：服务端（如代理隧道未建立时）挂住连接会导致命令无限卡死。
const defaultHTTPTimeout = 60 * time.Second

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

// Do 调用 POST {BaseURL}{path}，body 为 JSON object。
// 返回成功响应；code != 0 返回 *BizError，网络/协议错误返回 *LocalError。
// 命中 SyncInProgressCode（通讯录同步进行中）时按 SyncRetryInterval 等待重试，最多 SyncRetryAttempts 次。
func (c *Client) Do(ctx context.Context, path string, body map[string]any) (*Response, []byte, error) {
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

// do 单次请求实现（Do 的重试包装见上）。
func (c *Client) do(ctx context.Context, path string, body map[string]any) (*Response, []byte, error) {
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

	httpClient := c.httpClient()
	resp, err := httpClient.Do(req)
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

// Download 下载 url 内容（用于下载类命令的 --out）。
func (c *Client) Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &LocalError{Err: fmt.Errorf("build download request: %w", err)}
	}
	httpClient := c.httpClient()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &LocalError{Err: fmt.Errorf("download %s: %w", url, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &LocalError{Err: fmt.Errorf("download http %d", resp.StatusCode)}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &LocalError{Err: fmt.Errorf("read download body: %w", err)}
	}
	return data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
