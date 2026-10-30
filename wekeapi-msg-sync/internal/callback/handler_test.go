package callback

import "testing"

// splitGroupContent 是内部的纯函数，黑盒无法覆盖，测试留在本包。
func TestSplitGroupContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		sender  string
		text    string
		ok      bool
	}{
		{"标准 wxid 前缀", "wxid_abc:\n你好", "wxid_abc", "你好", true},
		{"CRLF 换行", "wxid_abc:\r\n你好", "wxid_abc", "你好", true},
		{"微信号前缀", "zhangsan88:\n你好", "zhangsan88", "你好", true},
		{"单行前缀", "zhangsan88:你好", "zhangsan88", "你好", true},
		{"前缀带空格", "张三 李四:\n你好", "", "张三 李四:\n你好", false},
		{"无冒号", "没有前缀的消息", "", "没有前缀的消息", false},
	}
	for _, c := range cases {
		sender, text, ok := splitGroupContent(c.content)
		if ok != c.ok || sender != c.sender || text != c.text {
			t.Errorf("%s: splitGroupContent(%q) = %q,%q,%v；期望 %q,%q,%v",
				c.name, c.content, sender, text, ok, c.sender, c.text, c.ok)
		}
	}
}
