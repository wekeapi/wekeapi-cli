package test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wekeapi-msg-sync/internal/callback"
	"wekeapi-msg-sync/internal/store"
)

func TestAddMsgDirect(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	payload := `{"appId":"app1","wxid":"wxid_self","msgFlag":"AddMsg","msgData":{
		"msg_type":1,
		"from_user_name":{"str":"wxid_peer"},
		"to_user_name":{"str":"wxid_self"},
		"content":{"str":"你好"},
		"new_msg_id":1001,
		"create_time":1700000000
	}}`
	if _, err := h.HandlePayload(ctx, []byte(payload)); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("消息 total=%d err=%v", page.Total, err)
	}
	m := page.Items[0]
	if m.FromWxID != "wxid_peer" || m.ToWxID != "wxid_self" || m.Content != "你好" || m.NewMsgID != "1001" {
		t.Errorf("消息字段错误: %+v", m)
	}

	// 同一 newMsgId 重复推送只入库一次。
	if _, err := h.HandlePayload(ctx, []byte(payload)); err != nil {
		t.Fatalf("重复回调处理失败: %v", err)
	}
	page, _ = st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", Page: 1, Limit: 10})
	if page.Total != 1 {
		t.Errorf("重复消息未去重，total=%d", page.Total)
	}
}

func TestAddMsgSelfSentAndGroup(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	selfSent := `{"appId":"app1","wxid":"wxid_self","msgFlag":"AddMsg","msgData":{
		"msg_type":1,
		"from_user_name":{"str":"wxid_self"},
		"to_user_name":{"str":"wxid_peer"},
		"content":{"str":"我发的"},
		"new_msg_id":1002,
		"create_time":1700000001
	}}`
	if _, err := h.HandlePayload(ctx, []byte(selfSent)); err != nil {
		t.Fatalf("处理自发消息失败: %v", err)
	}

	group := `{"appId":"app1","wxid":"wxid_self","msgFlag":"AddMsg","msgData":{
		"msg_type":1,
		"from_user_name":{"str":"123@chatroom"},
		"to_user_name":{"str":"wxid_self"},
		"content":{"str":"wxid_sender:\n群消息"},
		"new_msg_id":1003,
		"create_time":1700000002
	}}`
	if _, err := h.HandlePayload(ctx, []byte(group)); err != nil {
		t.Fatalf("处理群消息失败: %v", err)
	}

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", ConversationID: "S:wxid_peer", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("自发消息 total=%d err=%v", page.Total, err)
	}
	if page.Items[0].FromWxID != "wxid_self" || page.Items[0].ToWxID != "wxid_peer" {
		t.Errorf("自发消息字段错误: %+v", page.Items[0])
	}

	page, err = st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", ConversationID: "R:123@chatroom", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("群消息 total=%d err=%v", page.Total, err)
	}
	m := page.Items[0]
	if m.RoomID != "123@chatroom" || m.FromWxID != "wxid_sender" || m.Content != "群消息" {
		t.Errorf("群消息字段错误: %+v", m)
	}
}

func TestModContactAndDelContact(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	mod := `{"appId":"app1","msgFlag":"ModContact","msgData":{
		"userName":{"str":"wxid_new"},
		"nickName":{"str":"新朋友"},
		"remark":{"str":"客户"},
		"sex":1,
		"smallHeadImgUrl":"https://avatar"
	}}`
	if _, err := h.HandlePayload(ctx, []byte(mod)); err != nil {
		t.Fatalf("处理 ModContact 失败: %v", err)
	}
	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Keyword: "新朋友", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("联系人 total=%d err=%v", page.Total, err)
	}
	if page.Items[0].ContactType != store.TypeFriend || page.Items[0].Avatar != "https://avatar" {
		t.Errorf("联系人字段错误: %+v", page.Items[0])
	}

	// 空昵称不应覆盖已有昵称。
	mod2 := `{"appId":"app1","msgFlag":"ModContact","msgData":{
		"userName":{"str":"wxid_new"},
		"remark":{"str":"重点客户"}
	}}`
	if _, err := h.HandlePayload(ctx, []byte(mod2)); err != nil {
		t.Fatalf("处理 ModContact 更新失败: %v", err)
	}
	page, _ = st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Keyword: "新朋友", Page: 1, Limit: 10})
	if page.Total != 1 || page.Items[0].Remark != "重点客户" {
		t.Errorf("ModContact 更新结果错误: %+v", page.Items)
	}

	del := `{"appId":"app1","msgFlag":"DelContact","msgData":{"userName":{"str":"wxid_new"}}}`
	if _, err := h.HandlePayload(ctx, []byte(del)); err != nil {
		t.Fatalf("处理 DelContact 失败: %v", err)
	}
	page, _ = st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Page: 1, Limit: 10})
	if page.Total != 0 {
		t.Errorf("DelContact 后联系人 total=%d, want 0", page.Total)
	}
}

func TestAppIDMismatchAndUnknownFlag(t *testing.T) {
	ctx := context.Background()
	h, _ := newHandler(t)

	msg, err := h.HandlePayload(ctx, []byte(`{"appId":"other","msgFlag":"AddMsg","msgData":{}}`))
	if err != nil || !strings.Contains(msg, "appId mismatch") {
		t.Fatalf("appId 不匹配返回 msg=%q err=%v", msg, err)
	}
	msg, err = h.HandlePayload(ctx, []byte(`{"appId":"app1","msgFlag":"SomethingElse","msgData":{}}`))
	if err != nil || !strings.Contains(msg, "unsupported") {
		t.Fatalf("未知类型返回 msg=%q err=%v", msg, err)
	}
}

func TestInvalidPayload(t *testing.T) {
	ctx := context.Background()
	h, _ := newHandler(t)

	_, err := h.HandlePayload(ctx, []byte(`not-json`))
	if !errors.Is(err, callback.ErrInvalidPayload) {
		t.Fatalf("期望 ErrInvalidPayload，实际: %v", err)
	}
	_, err = h.HandlePayload(ctx, []byte(`{"appId":"app1","msgFlag":"AddMsg"}`))
	if !errors.Is(err, callback.ErrInvalidPayload) {
		t.Fatalf("空 msgData 期望 ErrInvalidPayload，实际: %v", err)
	}
}

// 群消息前缀是微信号时，按原样作为发送人。
func TestAddMsgGroupSenderAlias(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	payload := `{"appId":"app1","wxid":"wxid_self","msgFlag":"AddMsg","msgData":{
		"msg_type":1,"from_user_name":{"str":"123@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"ZhangSan88:\n大家好"},"new_msg_id":1051,"create_time":1700000100
	}}`
	sendCallback(t, h, payload)

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", ConversationID: "R:123@chatroom", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("群消息 total=%d err=%v", page.Total, err)
	}
	if m := page.Items[0]; m.FromWxID != "ZhangSan88" || m.Content != "大家好" {
		t.Errorf("微信号发送人解析错误: %+v", m)
	}
}

// wekeapi 新格式：msgFlag=ModContacts（复数）+ snake_case 字段。
func TestModContactsSnakeCase(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	sendCallback(t, h, `{"appId":"app1","msgFlag":"ModContacts","msgData":{
		"user_name":{"str":"wxid_snake"},
		"nick_name":{"str":"蛇形好友"},
		"remark":{"str":"蛇备注"},
		"alias":{"str":"snake"},
		"signature":{"str":"签名"},
		"sex":2,
		"small_head_img_url":"https://snake",
		"chatroom_status":0
	}}`)

	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Keyword: "蛇形好友", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("联系人 total=%d err=%v", page.Total, err)
	}
	c := page.Items[0]
	if c.ContactType != store.TypeFriend || c.Remark != "蛇备注" || c.Alias != "snake" ||
		c.Avatar != "https://snake" || c.Sex != 2 {
		t.Errorf("snake_case 联系人字段错误: %+v", c)
	}
}

// 企微联系人 ID（@openim）应归入 wework 类型。
func TestModContactsOpenIM(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	sendCallback(t, h, `{"appId":"app1","msgFlag":"ModContacts","msgData":{
		"user_name":{"str":"wm_abc@openim"},
		"nick_name":{"str":"企微好友"}
	}}`)

	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeWework, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || page.Items[0].ContactID != "wm_abc@openim" {
		t.Fatalf("企微联系人 = %+v, err=%v", page, err)
	}
}

// 新群 chatroom_status 非 0 时忽略；为 0 时入库。
func TestModContactsChatroomStatus(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	sendCallback(t, h, `{"appId":"app1","msgFlag":"ModContacts","msgData":{
		"user_name":{"str":"room_skip@chatroom"},
		"nick_name":{"str":"未加入的群"},
		"chatroom_status":1
	}}`)
	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeChatroom, Page: 1, Limit: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("未加入的群不应入库: total=%d err=%v", page.Total, err)
	}

	sendCallback(t, h, `{"appId":"app1","msgFlag":"ModContacts","msgData":{
		"user_name":{"str":"room_new@chatroom"},
		"nick_name":{"str":"新群"},
		"chatroom_status":0
	}}`)
	page, err = st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeChatroom, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || page.Items[0].ContactID != "room_new@chatroom" {
		t.Fatalf("新群入库结果 = %+v, err=%v", page, err)
	}
}

// 系统消息：扫码入群、邀请入群、解散群、被移出群，同时系统消息本身落库。
func TestSystemGroupEvents(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)
	roomCount := func() int {
		page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeChatroom, Page: 1, Limit: 50})
		if err != nil {
			t.Fatalf("查询群失败: %v", err)
		}
		return page.Total
	}

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10000,"from_user_name":{"str":"room_join@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"你通过扫描二维码加入群聊，群聊参与人还有：A、B"},"new_msg_id":8101,"create_time":1700200001
	}}`)
	if n := roomCount(); n != 1 {
		t.Fatalf("扫码入群后群数量 = %d, want 1", n)
	}

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10002,"from_user_name":{"str":"room_invite@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"<sysmsg type=\"sysmsgtemplate\">A[wxid_a]邀请你加入了群聊，群聊参与人还有：C、D</sysmsg>"},
		"new_msg_id":8102,"create_time":1700200002
	}}`)
	if n := roomCount(); n != 2 {
		t.Fatalf("邀请入群后群数量 = %d, want 2", n)
	}

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10002,"from_user_name":{"str":"room_join@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"<sysmsg type=\"sysmsgtemplate\">群主已解散该群聊</sysmsg>"},
		"new_msg_id":8103,"create_time":1700200003
	}}`)
	if n := roomCount(); n != 1 {
		t.Fatalf("解散群后群数量 = %d, want 1", n)
	}

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10000,"from_user_name":{"str":"room_invite@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"你被\"张三\"移出群聊"},"new_msg_id":8104,"create_time":1700200004
	}}`)
	if n := roomCount(); n != 0 {
		t.Fatalf("被移出群后群数量 = %d, want 0", n)
	}

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", Page: 1, Limit: 10})
	if err != nil || page.Total != 4 {
		t.Errorf("系统消息入库 total=%d err=%v, want 4", page.Total, err)
	}
}

// 企微好友添加通知（@openim）应新增 wework 联系人。
func TestWeworkAddSystemNotice(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10000,"from_user_name":{"str":"wm_abc@openim"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"你已添加了张三，现在可以开始聊天了。"},"new_msg_id":8201,"create_time":1700200005
	}}`)

	page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeWework, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || page.Items[0].ContactID != "wm_abc@openim" {
		t.Fatalf("企微好友 = %+v, err=%v", page, err)
	}
}

// 撤回系统消息应将原消息标记为 revoked。
func TestRevokeMessage(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":1,"from_user_name":{"str":"wxid_peer"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"待撤回消息"},"new_msg_id":8301,"create_time":1700200006
	}}`)
	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":10002,"from_user_name":{"str":"wxid_peer"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"<sysmsg type=\"revokemsg\"><revokemsg><session>wxid_peer</session><newmsgid>8301</newmsgid><replacemsg>\"x\" 撤回了一条消息</replacemsg></revokemsg></sysmsg>"},
		"new_msg_id":8302,"create_time":1700200007
	}}`)

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", Keyword: "待撤回消息", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("原消息 total=%d err=%v", page.Total, err)
	}
	if !page.Items[0].Revoked {
		t.Errorf("撤回事件未标记原消息: %+v", page.Items[0])
	}
}

// Offline 事件返回忽略，不落库。
func TestOfflineEvent(t *testing.T) {
	h, _ := newHandler(t)
	msg, err := h.HandlePayload(context.Background(),
		[]byte(`{"appId":"app1","wxid":"wxid_self","msgFlag":"Offline","msgData":{}}`))
	if err != nil || !strings.Contains(msg, "offline") {
		t.Fatalf("Offline 回调 msg=%q err=%v", msg, err)
	}
}

// 群消息来自本地还没有收录的群时，应把群增量写入联系人并异步补拉详情。
func TestAddMsgUnknownGroupEnsuresContact(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)
	h.SetDetailFetcher(func(ctx context.Context, contactID string) (*store.ContactDetail, bool) {
		return &store.ContactDetail{
			ContactID: contactID, Nickname: "新群", MemberCount: 5,
			RawJSON: `{"nickName":"新群"}`,
		}, true
	})

	sendCallback(t, h, `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":1,"from_user_name":{"str":"room_new@chatroom"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"wxid_sender:\n群里的第一条消息"},"new_msg_id":8401,"create_time":1700200008
	}}`)

	deadline := time.Now().Add(3 * time.Second)
	for {
		page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Type: store.TypeChatroom, Page: 1, Limit: 10})
		if err == nil && page.Total == 1 && page.Items[0].Nickname == "新群" {
			if page.Items[0].MemberCount != 5 {
				t.Errorf("群详情未补全: %+v", page.Items[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("新群未加入联系人: %+v (err=%v)", page.Items, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// 回调缺少 new_msg_id 时用内容哈希作为消息 ID，重复推送仍只入库一次。
func TestAddMsgWithoutNewMsgID(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)

	payload := `{"appId":"app1","msgFlag":"AddMsg","msgData":{
		"msg_type":1,"from_user_name":{"str":"wxid_peer"},"to_user_name":{"str":"wxid_self"},
		"content":{"str":"无ID消息"},"create_time":1700200009
	}}`
	sendCallback(t, h, payload)
	sendCallback(t, h, payload)

	page, err := st.QueryMessages(ctx, store.MessageFilter{Owner: "app1", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("无 new_msg_id 消息 total=%d err=%v, want 1", page.Total, err)
	}
	if !strings.HasPrefix(page.Items[0].NewMsgID, "h:") {
		t.Errorf("new_msg_id = %q, want h: 前缀的内容哈希", page.Items[0].NewMsgID)
	}
}

// 新联系人应异步补拉详情（昵称/标签）。
func TestModContactDetailFetcher(t *testing.T) {
	ctx := context.Background()
	h, st := newHandler(t)
	h.SetDetailFetcher(func(ctx context.Context, contactID string) (*store.ContactDetail, bool) {
		return &store.ContactDetail{
			ContactID: contactID, Nickname: "详情昵称", Labels: "1,2",
			RawJSON: `{"nickName":"详情昵称"}`, MemberCount: 3,
		}, true
	})

	sendCallback(t, h, `{"appId":"app1","msgFlag":"ModContacts","msgData":{
		"user_name":{"str":"wxid_detail"},"nick_name":{"str":"回调昵称"}
	}}`)

	deadline := time.Now().Add(3 * time.Second)
	for {
		page, err := st.QueryContacts(ctx, store.ContactFilter{Owner: "app1", Keyword: "详情昵称", Page: 1, Limit: 5})
		if err == nil && page.Total == 1 {
			if page.Items[0].Labels != "1,2" {
				t.Errorf("详情标签未写入: %+v", page.Items[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("异步补拉详情超时")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
