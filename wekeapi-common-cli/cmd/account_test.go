package cmd

import (
	"encoding/json"
	"testing"
)

// TestExtractAccountInfo login status 在线时的顶层扁平结构提取。
func TestExtractAccountInfo(t *testing.T) {
	data := map[string]any{
		"onlineStatus": json.Number("1"),
		"wxid":         "wxid_self",
		"alias":        "myalias",
		"nickname":     "我自己",
		"headImgUrl":   "http://img/top.jpg",
	}
	acc := extractAccountInfo(data)
	if acc == nil {
		t.Fatalf("应能提取到账号信息")
	}
	if acc.WxID != "wxid_self" || acc.Alias != "myalias" || acc.Nickname != "我自己" {
		t.Fatalf("扁平结构字段不符: %s/%s/%s", acc.WxID, acc.Alias, acc.Nickname)
	}
	if acc.HeadImgURL != "http://img/top.jpg" {
		t.Fatalf("headImgUrl 不符: %q", acc.HeadImgURL)
	}
	if acc.Mobile != "" || acc.Uin != "" {
		t.Fatalf("status 响应无 mobile/uin，应为空: %s/%s", acc.Mobile, acc.Uin)
	}
}

// TestExtractAccountInfoNoWxid 响应无 wxid（离线/未登录）时返回 nil。
func TestExtractAccountInfoNoWxid(t *testing.T) {
	cases := []map[string]any{
		{"onlineStatus": json.Number("0")},           // 未登录：无个人信息
		{"accountInfo": map[string]any{"wxid": "x"}}, // 嵌套结构不再兼容：只认顶层 wxid
		{}, // 空响应
	}
	for i, data := range cases {
		if acc := extractAccountInfo(data); acc != nil {
			t.Fatalf("case %d：无顶层 wxid 应返回 nil，实际 %+v", i, acc)
		}
	}
}
