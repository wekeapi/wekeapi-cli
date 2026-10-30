package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ownerParam 解析账号归属参数：app_id（兼容 appId），缺省使用配置 appId。
// 单账号服务，显式传入其他 appId 视为参数错误。
func (s *Server) ownerParam(r *http.Request) (string, error) {
	v := strings.TrimSpace(r.URL.Query().Get("app_id"))
	if v == "" {
		v = strings.TrimSpace(r.URL.Query().Get("appId"))
	}
	if v == "" {
		return s.cfg.AppID, nil
	}
	if v != s.cfg.AppID {
		return "", fmt.Errorf("app_id %q 与本服务配置的 appId 不一致", v)
	}
	return v, nil
}

// pageLimit 解析分页参数，默认 page=1、limit=20，limit 上限 100。
func pageLimit(r *http.Request) (page, limit int, err error) {
	page, err = intParam(r, "page", 1)
	if err != nil {
		return 0, 0, err
	}
	if page < 1 {
		return 0, 0, fmt.Errorf("page 必须大于等于 1")
	}
	limit, err = intParam(r, "limit", 20)
	if err != nil {
		return 0, 0, err
	}
	if limit < 1 || limit > 100 {
		return 0, 0, fmt.Errorf("limit 必须在 1-100 之间")
	}
	return page, limit, nil
}

func intParam(r *http.Request, name string, def int) (int, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是整数", name)
	}
	return n, nil
}

// contactTypeParam 解析联系人类型参数。
func contactTypeParam(r *http.Request) (int, error) {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	switch v {
	case "", "all":
		return 0, nil
	case "friend", "friends":
		return 1, nil
	case "chatroom", "room", "rooms":
		return 2, nil
	case "gh", "ghs":
		return 3, nil
	case "wework":
		return 4, nil
	default:
		return 0, fmt.Errorf("type 只支持 friend/chatroom/gh/wework/all，当前为 %q", v)
	}
}

// labelParams 解析标签过滤参数：支持 ?label=标签名 重复传参与逗号分隔。
func labelParams(r *http.Request) []string {
	var out []string
	for _, raw := range r.URL.Query()["label"] {
		for _, part := range strings.Split(raw, ",") {
			if v := strings.TrimSpace(part); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// parseTimeParam 解析时间参数，支持 RFC3339、2006-01-02 15:04:05、2006-01-02 与 Unix 秒。
func parseTimeParam(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	if isDigits(v) {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("时间参数 %q 不是合法的 Unix 时间戳", v)
		}
		return n, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("时间参数 %q 格式错误，支持 RFC3339 或 Unix 秒", v)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
