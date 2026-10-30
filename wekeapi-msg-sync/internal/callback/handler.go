// Package callback 解析消息回调（AddMsg/ModContacts/DelContact/Offline）并写入存储。
//
// 字段采用 wekeapi 官方格式：msgFlag/msgData 信封；AddMsg 为 snake_case
// （msg_type / from_user_name.str / content.str …），ModContact/DelContact 为
// camelCase（userName.str / nickName.str …）。
//
// 除消息与联系人变更外，还会从系统消息中识别入群/退群/解散群、企微好友添加与撤回事件。
package callback

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"wekeapi-msg-sync/internal/store"
)

// ErrInvalidPayload 表示回调数据格式错误（HTTP 400，不重试）。
var ErrInvalidPayload = errors.New("invalid callback payload")

// 系统消息类型。
const (
	msgTypeSystemNotice = 10000 // 系统通知（入群/退群/企微好友等）
	msgTypeSystem       = 10002 // 系统消息（sysmsg XML，含撤回）
)

// 系统事件文本匹配规则（与参考实现保持一致）。
var (
	systemDissolvePattern = regexp.MustCompile(`群主.*已解散该群聊`)
	systemInvitePattern   = regexp.MustCompile(`A\[.*邀请你加入了群聊，群聊参与人还有：`)
	kickedPattern         = regexp.MustCompile(`^你被".*"移出群聊$`)
	qwAddedPattern        = regexp.MustCompile(`^你已添加了.*，现在可以开始聊天了。$`)
)

// Logf 日志函数。
type Logf func(format string, args ...any)

// DetailFetcher 拉取单个联系人详情（由调用方注入，避免 callback 依赖 client）。
type DetailFetcher func(ctx context.Context, contactID string) (*store.ContactDetail, bool)

// Handler 回调处理器。
type Handler struct {
	st    *store.Store
	appID string
	wxid  string
	logf  Logf

	detailFetcher DetailFetcher
	detailMu      sync.Mutex
	lastDetailReq map[string]time.Time
}

// New 创建回调处理器。
func New(st *store.Store, appID, wxid string, logf Logf) *Handler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Handler{
		st:            st,
		appID:         appID,
		wxid:          wxid,
		logf:          logf,
		lastDetailReq: make(map[string]time.Time),
	}
}

// SetDetailFetcher 注入联系人详情拉取函数：ModContacts 后异步补全昵称/标签等字段。
func (h *Handler) SetDetailFetcher(fn DetailFetcher) { h.detailFetcher = fn }

// envelope 回调信封（wekeapi 官方格式 msgFlag/msgData）。
type envelope struct {
	AppID   string          `json:"appId"`
	WxID    string          `json:"wxid"`
	MsgFlag string          `json:"msgFlag"`
	MsgData json.RawMessage `json:"msgData"`
}

// HandlePayload 处理一次回调，返回响应中的 message。
func (h *Handler) HandlePayload(ctx context.Context, raw []byte) (string, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	appID := strings.TrimSpace(env.AppID)
	if appID != "" && !strings.EqualFold(appID, h.appID) {
		h.logf("忽略 appId=%s 的回调（本服务配置 appId=%s）", appID, h.appID)
		return "Ignored: appId mismatch", nil
	}

	data := bytes.TrimSpace(env.MsgData)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return "", fmt.Errorf("%w: msgData 为空", ErrInvalidPayload)
	}

	selfWxID := strings.TrimSpace(firstNonEmpty(env.WxID, h.wxid))

	flag := env.MsgFlag
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "addmsg":
		if err := h.handleAddMsg(ctx, data); err != nil {
			return "", err
		}
		return "Callback processed", nil
	case "modcontact", "modcontacts":
		if err := h.handleModContact(ctx, data); err != nil {
			return "", err
		}
		return "Callback processed", nil
	case "delcontact", "delcontacts":
		if err := h.handleDelContact(ctx, data); err != nil {
			return "", err
		}
		return "Callback processed", nil
	case "offline":
		h.logf("收到 Offline 回调（wxid=%s），仅记录状态", selfWxID)
		return "Ignored: offline event", nil
	default:
		h.logf("忽略未知回调类型 msgFlag=%q", flag)
		return "Ignored: unsupported msgFlag " + flag, nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// strField 兼容字符串与 {"str":"..."} 两种字段形态。
type strField string

func (s *strField) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}
	if trimmed[0] == '"' {
		var v string
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return err
		}
		*s = strField(v)
		return nil
	}
	var obj struct {
		Str string `json:"str"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return err
	}
	*s = strField(obj.Str)
	return nil
}

// numField 兼容数字与字符串两种数字形态，保留大整数原始文本。
type numField string

func (n *numField) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		*n = ""
		return nil
	}
	if len(trimmed) >= 2 && trimmed[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*n = numField(v)
		return nil
	}
	*n = numField(trimmed)
	return nil
}

func (n numField) Int64() int64 {
	s := strings.TrimSpace(string(n))
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func firstNum(values ...numField) numField {
	for _, v := range values {
		if strings.TrimSpace(string(v)) != "" {
			return v
		}
	}
	return ""
}

// addMsg 归一化后的 AddMsg 数据。
type addMsg struct {
	MsgType    numField
	From       strField
	To         strField
	Content    strField
	NewMsgID   numField
	CreateTime numField
}

type addMsgWire struct {
	MsgType    numField `json:"msg_type"`
	From       strField `json:"from_user_name"`
	To         strField `json:"to_user_name"`
	Content    strField `json:"content"`
	NewMsgID   numField `json:"new_msg_id"`
	MsgID      numField `json:"msg_id"`
	CreateTime numField `json:"create_time"`
}

func parseAddMsg(data []byte) (*addMsg, error) {
	var w addMsgWire
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&w); err != nil {
		return nil, err
	}
	return &addMsg{
		MsgType:    w.MsgType,
		From:       w.From,
		To:         w.To,
		Content:    w.Content,
		NewMsgID:   firstNum(w.NewMsgID, w.MsgID),
		CreateTime: w.CreateTime,
	}, nil
}

func (h *Handler) handleAddMsg(ctx context.Context, data []byte) error {
	msg, err := parseAddMsg(data)
	if err != nil {
		return fmt.Errorf("%w: AddMsg 解析失败: %v", ErrInvalidPayload, err)
	}

	rawFrom := string(msg.From)
	rawTo := string(msg.To)
	if rawFrom == "" && rawTo == "" {
		return fmt.Errorf("%w: AddMsg 缺少 from_user_name/to_user_name", ErrInvalidPayload)
	}

	roomID := ""
	if strings.HasSuffix(rawFrom, "@chatroom") {
		roomID = rawFrom
	} else if strings.HasSuffix(rawTo, "@chatroom") {
		roomID = rawTo
	}

	content := string(msg.Content)
	from := rawFrom
	msgType := int(msg.MsgType.Int64())
	isSystem := msgType == msgTypeSystemNotice || msgType == msgTypeSystem

	if isSystem {
		h.handleSystemEvent(ctx, msgType, rawFrom, rawTo, content)
	} else {
		if roomID != "" {
			// 群消息可能来自本地还没有收录的群：按群增量写入联系人并异步补拉详情。
			h.ensureContact(ctx, store.TypeChatroom, roomID)
		}
		if roomID != "" && rawFrom == roomID {
			// 接收到的群消息带“发送人:\n内容”前缀，自己发的没有。
			if sender, text, ok := splitGroupContent(content); ok {
				from = sender
				content = text
			}
		}
	}

	createTime := msg.CreateTime.Int64()
	if createTime <= 0 {
		createTime = time.Now().Unix()
	}

	rec := &store.MessageRecord{
		NewMsgID:   normalizeMsgID(string(msg.NewMsgID)),
		RoomID:     roomID,
		CreateTime: createTime,
		MsgType:    msgType,
		FromWxID:   from,
		ToWxID:     rawTo,
		Content:    content,
		RawJSON:    sanitizeMsgData(data),
	}

	inserted, err := h.st.InsertMessage(ctx, h.appID, rec)
	if err != nil {
		return err
	}
	if !inserted {
		h.logf("重复消息已忽略（newMsgId=%s）", rec.NewMsgID)
	}
	return nil
}

// modContact 归一化后的 ModContacts 数据。
type modContact struct {
	ContactID      string
	Nickname       string
	Alias          string
	Remark         string
	Signature      string
	Avatar         string
	Sex            int
	ContactType    int
	ChatroomStatus int
	DeleteFlag     int
}

type modContactWire struct {
	UserName             strField `json:"userName"`
	UserNameSnake        strField `json:"user_name"`
	NickName             strField `json:"nickName"`
	NickNameSnake        strField `json:"nick_name"`
	Remark               strField `json:"remark"`
	Alias                strField `json:"alias"`
	Signature            strField `json:"signature"`
	Sex                  numField `json:"sex"`
	SmallHeadImgURL      string   `json:"smallHeadImgUrl"`
	SmallHeadImgURLSnake string   `json:"small_head_img_url"`
	BigHeadImgURL        string   `json:"bigHeadImgUrl"`
	BigHeadImgURLSnake   string   `json:"big_head_img_url"`
	ContactType          numField `json:"contact_type"`
	ChatroomStatus       numField `json:"chatroomStatus"`
	ChatroomStatusSnake  numField `json:"chatroom_status"`
	DeleteFlag           numField `json:"deleteFlag"`
	DeleteFlagSnake      numField `json:"delete_flag"`
}

func parseModContact(data []byte) (*modContact, error) {
	var w modContactWire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	avatar := strings.TrimSpace(firstNonEmpty(w.SmallHeadImgURL, w.SmallHeadImgURLSnake))
	if avatar == "" {
		avatar = strings.TrimSpace(firstNonEmpty(w.BigHeadImgURL, w.BigHeadImgURLSnake))
	}
	return &modContact{
		ContactID:      strings.TrimSpace(firstNonEmpty(string(w.UserName), string(w.UserNameSnake))),
		Nickname:       firstNonEmpty(string(w.NickName), string(w.NickNameSnake)),
		Alias:          string(w.Alias),
		Remark:         string(w.Remark),
		Signature:      string(w.Signature),
		Avatar:         avatar,
		Sex:            int(w.Sex.Int64()),
		ContactType:    int(w.ContactType.Int64()),
		ChatroomStatus: int(firstNum(w.ChatroomStatus, w.ChatroomStatusSnake).Int64()),
		DeleteFlag:     int(firstNum(w.DeleteFlag, w.DeleteFlagSnake).Int64()),
	}, nil
}

func (h *Handler) handleModContact(ctx context.Context, data []byte) error {
	mc, err := parseModContact(data)
	if err != nil {
		return fmt.Errorf("%w: ModContacts 解析失败: %v", ErrInvalidPayload, err)
	}
	contactID := mc.ContactID
	if contactID == "" {
		return fmt.Errorf("%w: ModContacts 缺少 userName", ErrInvalidPayload)
	}

	contactType := store.TypeFriend
	existing, ok, err := h.st.ContactType(ctx, h.appID, contactID)
	if err != nil {
		return err
	}
	if ok {
		contactType = existing
	}
	switch {
	case strings.HasSuffix(contactID, "@chatroom"):
		contactType = store.TypeChatroom
	case strings.HasSuffix(contactID, "@openim"):
		contactType = store.TypeWework
	}

	// 新群且状态非 0（未加入/已退出）不入库，与参考实现一致。
	if !ok && contactType == store.TypeChatroom && mc.ChatroomStatus != 0 {
		h.logf("忽略未加入的群 %s（chatroom_status=%d）", contactID, mc.ChatroomStatus)
		return nil
	}

	detail := &store.ContactDetail{
		ContactID: contactID,
		Nickname:  mc.Nickname,
		Alias:     mc.Alias,
		Avatar:    mc.Avatar,
		Remark:    mc.Remark,
		Sex:       mc.Sex,
		Signature: mc.Signature,
		RawJSON:   string(data),
	}

	if !ok && contactDetailEmpty(detail) {
		// 只有 ID 的回调先落骨架，详情由下一轮同步补齐。
		if err := h.st.InsertSkeletons(ctx, h.appID, contactType, []string{contactID}); err != nil {
			return err
		}
	} else if err := h.st.UpsertContactFromMod(ctx, h.appID, contactType, detail); err != nil {
		return err
	}

	// 新联系人/资料变更后异步补拉详情（昵称、标签等），避免回调阻塞。
	h.fetchDetailAsync(contactType, contactID)
	return nil
}

func contactDetailEmpty(d *store.ContactDetail) bool {
	return d.Nickname == "" && d.Alias == "" && d.Avatar == "" && d.Remark == "" &&
		d.Signature == "" && d.Sex == 0
}

// delContact DelContact 回调数据。
type delContact struct {
	UserName      strField `json:"userName"`
	UserNameSnake strField `json:"user_name"`
}

func (h *Handler) handleDelContact(ctx context.Context, data []byte) error {
	var dc delContact
	if err := json.Unmarshal(data, &dc); err != nil {
		return fmt.Errorf("%w: DelContact 解析失败: %v", ErrInvalidPayload, err)
	}
	contactID := strings.TrimSpace(firstNonEmpty(string(dc.UserName), string(dc.UserNameSnake)))
	if contactID == "" {
		return fmt.Errorf("%w: DelContact 缺少 userName", ErrInvalidPayload)
	}
	return h.st.DeleteContact(ctx, h.appID, contactID)
}

// revokeSysMsg 撤回系统消息 XML。
type revokeSysMsg struct {
	XMLName xml.Name `xml:"sysmsg"`
	Type    string   `xml:"type,attr"`
	Revoke  struct {
		NewMsgID int64 `xml:"newmsgid"`
	} `xml:"revokemsg"`
}

// handleSystemEvent 处理系统消息中的增量事件：撤回、入群/退群/解散群、企微好友添加。
// 事件处理为尽力而为：失败只记日志，不影响消息落库。
func (h *Handler) handleSystemEvent(ctx context.Context, msgType int, from, to, content string) {
	if strings.Contains(content, `type="revokemsg"`) {
		h.handleRevoke(ctx, content)
	}

	roomID := from
	if !strings.HasSuffix(roomID, "@chatroom") {
		roomID = to
	}

	switch msgType {
	case msgTypeSystem: // 10002 sysmsg XML
		if strings.Contains(content, "<sysmsg") && strings.Contains(content, `type="sysmsgtemplate"`) {
			switch {
			case systemDissolvePattern.MatchString(content):
				h.removeRoom(ctx, roomID)
			case systemInvitePattern.MatchString(content), strings.Contains(content, "邀请你加入了群聊，群聊参与人还有"):
				h.ensureContact(ctx, store.TypeChatroom, roomID)
			}
		}
	case msgTypeSystemNotice: // 10000 系统通知
		switch {
		case strings.HasPrefix(content, "你通过扫描二维码加入群聊，群聊参与人还有："):
			h.ensureContact(ctx, store.TypeChatroom, roomID)
		case content == "你已解散该群聊":
			h.removeRoom(ctx, roomID)
		case kickedPattern.MatchString(content):
			h.removeRoom(ctx, roomID)
		}
	}

	// 企微好友添加通知（from 为企微联系人 ID）。
	if strings.HasSuffix(from, "@openim") && qwAddedPattern.MatchString(content) {
		h.ensureContact(ctx, store.TypeWework, from)
	}
}

func (h *Handler) handleRevoke(ctx context.Context, content string) {
	if idx := strings.Index(content, "<sysmsg"); idx > 0 {
		content = content[idx:]
	}
	var msg revokeSysMsg
	if err := xml.Unmarshal([]byte(content), &msg); err != nil || msg.Revoke.NewMsgID <= 0 {
		h.logf("撤回消息解析失败: %v", err)
		return
	}
	affected, err := h.st.MarkMessageRevoked(ctx, h.appID, strconv.FormatInt(msg.Revoke.NewMsgID, 10))
	if err != nil {
		h.logf("标记消息撤回失败: %v", err)
		return
	}
	h.logf("消息已标记撤回（newMsgId=%d，影响 %d 条）", msg.Revoke.NewMsgID, affected)
}

// ensureContact 确保联系人在本地存在：不存在时写入骨架并异步补拉详情。
// 群消息来自本地还没有的群时，靠这里把群增量加入联系人。
func (h *Handler) ensureContact(ctx context.Context, contactType int, contactID string) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return
	}
	if _, ok, err := h.st.ContactType(ctx, h.appID, contactID); err != nil {
		h.logf("查询联系人 %s 失败: %v", contactID, err)
		return
	} else if ok {
		return
	}
	if err := h.st.InsertSkeletons(ctx, h.appID, contactType, []string{contactID}); err != nil {
		h.logf("写入联系人骨架失败（%s）: %v", contactID, err)
		return
	}
	h.logf("新增联系人 %s（类型 %d），异步补拉详情", contactID, contactType)
	h.fetchDetailAsync(contactType, contactID)
}

func (h *Handler) removeRoom(ctx context.Context, roomID string) {
	roomID = strings.TrimSpace(roomID)
	if !strings.HasSuffix(roomID, "@chatroom") {
		return
	}
	if err := h.st.DeleteContact(ctx, h.appID, roomID); err != nil {
		h.logf("删除群 %s 失败: %v", roomID, err)
		return
	}
	h.logf("群 %s 已解散/退出，删除本地记录", roomID)
}

// fetchDetailAsync 异步补拉联系人详情；3 秒内同一联系人只触发一次（防回调风暴）。
func (h *Handler) fetchDetailAsync(contactType int, contactID string) {
	if h.detailFetcher == nil || contactType == store.TypeWework {
		return
	}
	h.detailMu.Lock()
	if t, ok := h.lastDetailReq[contactID]; ok && time.Since(t) < 3*time.Second {
		h.detailMu.Unlock()
		return
	}
	if len(h.lastDetailReq) > 4096 {
		h.lastDetailReq = make(map[string]time.Time)
	}
	h.lastDetailReq[contactID] = time.Now()
	h.detailMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		detail, ok := h.detailFetcher(ctx, contactID)
		if !ok || detail == nil {
			return
		}
		detail.ContactID = contactID
		if err := h.st.UpsertContactDetail(context.Background(), h.appID, contactType, detail); err != nil {
			h.logf("回调后补拉联系人详情失败（%s）: %v", contactID, err)
		}
	}()
}

// splitGroupContent 解析群消息内容中的发送人前缀（标准格式 "发送人:\n内容"）。
// 前缀可能是 wxid，也可能是微信号，按原样作为发送人；兼容 CRLF 换行。
func splitGroupContent(content string) (sender, text string, ok bool) {
	i := strings.Index(content, ":")
	if i <= 0 {
		return "", content, false
	}
	candidate := content[:i]
	if !isLikelySender(candidate) {
		return "", content, false
	}
	text = strings.TrimPrefix(content[i+1:], "\r\n")
	text = strings.TrimPrefix(text, "\n")
	return candidate, text, true
}

func isLikelySender(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == ':' || r == '<' || r == '>' {
			return false
		}
	}
	return true
}

// sanitizeMsgData 移除 img_buf/ImgBuf（可能包含 base64 缩略图），保留其余原始字段。
func sanitizeMsgData(data []byte) string {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return string(data)
	}
	delete(m, "img_buf")
	b, err := json.Marshal(m)
	if err != nil {
		return string(data)
	}
	return string(b)
}

// normalizeMsgID 把消息 ID 规范为十进制字符串；"0" 视为缺失。
func normalizeMsgID(v string) string {
	s := strings.TrimSpace(v)
	if s == "" || s == "0" {
		return ""
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return s
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.FormatFloat(f, 'f', 0, 64)
	}
	return s
}
