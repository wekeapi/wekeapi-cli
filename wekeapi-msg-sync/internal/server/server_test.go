package server

import (
	"testing"

	"wekeapi-msg-sync/internal/store"
)

// conversationIDOf 是内部函数，黑盒无法覆盖，测试留在本包。
func TestConversationIDOf(t *testing.T) {
	const self = "wxid_self"
	cases := []struct {
		name string
		self string
		m    store.Message
		want string
	}{
		{"群消息", self, store.Message{RoomID: "123@chatroom", FromWxID: "wxid_a", ToWxID: self}, "R:123@chatroom"},
		{"自己发出的私聊", self, store.Message{FromWxID: self, ToWxID: "wxid_peer"}, "S:wxid_peer"},
		{"收到的私聊", self, store.Message{FromWxID: "wxid_peer", ToWxID: self}, "S:wxid_peer"},
		{"缺少本账号 wxid 时按发送人归会话", "", store.Message{FromWxID: self, ToWxID: "wxid_peer"}, "S:wxid_self"},
	}
	for _, c := range cases {
		if got := conversationIDOf(c.m, c.self); got != c.want {
			t.Errorf("%s: conversationIDOf = %q, want %q", c.name, got, c.want)
		}
	}
}
