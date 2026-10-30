package cmd

import (
	"strings"
	"testing"
)

// TestSentMessageCommandsHavePostRun 所有发送/转发命令都必须挂上落库钩子与消息类型映射。
func TestSentMessageCommandsHavePostRun(t *testing.T) {
	for _, spec := range messageSpecs {
		if !strings.HasPrefix(spec.Path, "/v1/message/send") &&
			!strings.HasPrefix(spec.Path, "/v1/message/forward") {
			continue
		}
		if spec.PostRun == nil {
			t.Errorf("%s（%s）未挂载 PostRun，发出的消息不会写入数据同步服务", spec.Use, spec.Path)
		}
		if _, ok := sentMsgTypes[spec.Use]; !ok {
			t.Errorf("%s 缺少 sentMsgTypes 映射", spec.Use)
		}
	}
}

func TestSentContent(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"文本", map[string]any{"content": "你好"}, "你好"},
		{"链接", map[string]any{"title": "标题", "linkUrl": "https://example.com"}, "标题 https://example.com"},
		{"文件", map[string]any{"fileName": "报告.pdf", "fileData": "QUJD"}, "报告.pdf"},
		{"名片", map[string]any{"cardNickname": "张三", "cardWxid": "wxid_zhangsan"}, "张三"},
		{"图片不落 base64", map[string]any{"imageData": "QUJD", "toWxid": "wxid_a"}, ""},
	}
	for _, c := range cases {
		if got := sentContent(c.body); got != c.want {
			t.Errorf("%s: sentContent = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSentRawJSON(t *testing.T) {
	if got := sentRawJSON(nil); got != "" {
		t.Errorf("空响应应返回空串，实际 %q", got)
	}
	got := sentRawJSON(map[string]any{"newMsgId": "2001", "createTime": "1700000200"})
	if !strings.Contains(got, "2001") {
		t.Errorf("raw JSON 应保留响应字段: %s", got)
	}
}
