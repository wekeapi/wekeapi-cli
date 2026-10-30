package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wekeapi-msg-sync/internal/callback"
	"wekeapi-msg-sync/internal/store"
)

const (
	maxCallbackBody = 32 << 20 // 32MB
	maxRecordBody   = 1 << 20  // 1MB：发送消息记录（不含文件内容）
)

type contactItem struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Nickname  string   `json:"nickname"`
	Remark    string   `json:"remark"`
	Alias     string   `json:"alias"`
	Avatar    string   `json:"avatar"`
	Type      string   `json:"type"`
	Sex       int      `json:"sex"`
	LabelIDs  []int64  `json:"label_ids"`
	Labels    []string `json:"labels"`
	UpdatedAt string   `json:"updated_at"`
}

type roomItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Remark      string `json:"remark"`
	Avatar      string `json:"avatar"`
	Owner       string `json:"owner"`
	MemberCount int    `json:"member_count"`
	UpdatedAt   string `json:"updated_at"`
}

type messageItem struct {
	ID             int64  `json:"id"`
	NewMsgID       string `json:"new_msg_id"`
	ConversationID string `json:"conversation_id"`
	From           string `json:"from"`
	To             string `json:"to"`
	RoomID         string `json:"room_id,omitempty"`
	Content        string `json:"content"`
	Timestamp      string `json:"timestamp"`
	CreateTime     int64  `json:"create_time"`
	Type           string `json:"type"`
	MsgType        int    `json:"msg_type"`
	Revoked        bool   `json:"revoked"`
}

type syncStatusData struct {
	AppID         string `json:"app_id"`
	WxID          string `json:"wxid"`
	Status        string `json:"status"`
	LastSyncTime  string `json:"last_sync_time"`
	LastError     string `json:"last_error,omitempty"`
	ContactsCount int    `json:"contacts_count"`
	MessagesCount int    `json:"messages_count"`
	RoomsCount    int    `json:"rooms_count"`
	DBDriver      string `json:"db_driver"`
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	owner, err := s.ownerParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	contactType, err := contactTypeParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	page, limit, err := pageLimit(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}

	// 标签过滤：标签名或标签 ID（可逗号分隔多个，多个表示同时具备）。
	var labelIDs []int64
	if refs := labelParams(r); len(refs) > 0 {
		labelIDs, err = s.resolveLabelIDs(r.Context(), owner, refs)
		if err != nil {
			var nf *labelNotFoundError
			if errors.As(err, &nf) {
				s.badRequest(w, err)
			} else {
				s.internalError(w, "解析标签失败", err)
			}
			return
		}
	}

	pageData, err := s.st.QueryContacts(r.Context(), store.ContactFilter{
		Owner:   owner,
		Type:    contactType,
		Keyword: r.URL.Query().Get("keyword"),
		Labels:  labelIDs,
		Page:    page,
		Limit:   limit,
	})
	if err != nil {
		s.internalError(w, "查询联系人失败", err)
		return
	}

	dict, err := s.labelDict(r, owner)
	if err != nil {
		s.internalError(w, "查询标签失败", err)
		return
	}
	items := make([]contactItem, 0, len(pageData.Items))
	for _, c := range pageData.Items {
		ids, names := labelIDsAndNames(c.Labels, dict)
		name := c.Nickname
		if name == "" {
			name = c.Remark
		}
		items = append(items, contactItem{
			ID:        c.ContactID,
			Name:      name,
			Nickname:  c.Nickname,
			Remark:    c.Remark,
			Alias:     c.Alias,
			Avatar:    c.Avatar,
			Type:      contactTypeName(c.ContactType),
			Sex:       c.Sex,
			LabelIDs:  ids,
			Labels:    names,
			UpdatedAt: formatDBTime(c.UpdatedAt),
		})
	}
	writeSuccess(w, map[string]any{
		"contacts": items,
		"total":    pageData.Total,
		"page":     page,
		"limit":    limit,
	})
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	owner, err := s.ownerParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	page, limit, err := pageLimit(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	pageData, err := s.st.QueryContacts(r.Context(), store.ContactFilter{
		Owner:   owner,
		Type:    store.TypeChatroom,
		Keyword: r.URL.Query().Get("keyword"),
		Page:    page,
		Limit:   limit,
	})
	if err != nil {
		s.internalError(w, "查询群聊失败", err)
		return
	}
	items := make([]roomItem, 0, len(pageData.Items))
	for _, c := range pageData.Items {
		name := c.Nickname
		if name == "" {
			name = c.Remark
		}
		items = append(items, roomItem{
			ID:          c.ContactID,
			Name:        name,
			Remark:      c.Remark,
			Avatar:      c.Avatar,
			Owner:       "",
			MemberCount: c.MemberCount,
			UpdatedAt:   formatDBTime(c.UpdatedAt),
		})
	}
	writeSuccess(w, map[string]any{
		"rooms": items,
		"total": pageData.Total,
		"page":  page,
		"limit": limit,
	})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	owner, err := s.ownerParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	page, limit, err := pageLimit(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	startTime, err := parseTimeParam(r.URL.Query().Get("start_time"))
	if err != nil {
		s.badRequest(w, err)
		return
	}
	endTime, err := parseTimeParam(r.URL.Query().Get("end_time"))
	if err != nil {
		s.badRequest(w, err)
		return
	}
	if startTime > 0 && endTime > 0 && startTime > endTime {
		s.badRequest(w, fmt.Errorf("start_time 不能大于 end_time"))
		return
	}
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	if conversationID != "" {
		if !strings.HasPrefix(conversationID, "S:") && !strings.HasPrefix(conversationID, "R:") {
			s.badRequest(w, fmt.Errorf("conversation_id 格式错误：私聊 S:<wxid>，群聊 R:<roomid>"))
			return
		}
		if strings.TrimSpace(conversationID[2:]) == "" {
			s.badRequest(w, fmt.Errorf("conversation_id 缺少 wxid/roomid"))
			return
		}
	}

	pageData, err := s.st.QueryMessages(r.Context(), store.MessageFilter{
		Owner:          owner,
		ConversationID: conversationID,
		Keyword:        r.URL.Query().Get("keyword"),
		StartTime:      startTime,
		EndTime:        endTime,
		Page:           page,
		Limit:          limit,
	})
	if err != nil {
		s.internalError(w, "查询消息失败", err)
		return
	}

	items := make([]messageItem, 0, len(pageData.Items))
	for _, m := range pageData.Items {
		items = append(items, messageItem{
			ID:             m.ID,
			NewMsgID:       m.NewMsgID,
			ConversationID: conversationIDOf(m, s.cfg.WxID),
			From:           m.FromWxID,
			To:             m.ToWxID,
			RoomID:         m.RoomID,
			Content:        m.Content,
			Timestamp:      formatUnixTime(m.CreateTime),
			CreateTime:     m.CreateTime,
			Type:           messageTypeName(m.MsgType),
			MsgType:        m.MsgType,
			Revoked:        m.Revoked,
		})
	}
	writeSuccess(w, map[string]any{
		"messages": items,
		"total":    pageData.Total,
		"page":     page,
		"limit":    limit,
	})
}

// createMessageRequest 本账号已发出消息的入库请求（由 CLI 发送成功后上报）。
type createMessageRequest struct {
	NewMsgID   string `json:"new_msg_id"`
	ToWxID     string `json:"to_wxid"`
	RoomID     string `json:"room_id"`
	MsgType    int    `json:"msg_type"`
	CreateTime int64  `json:"create_time"`
	Content    string `json:"content"`
	RawJSON    string `json:"raw_json"`
}

// handleCreateMessage 记录一条本账号发出的消息：from_wxid 取服务配置的 wxid，
// 收发方向因此不需单独存列；按 (wx_id, new_msg_id) 幂等。
func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	owner, err := s.ownerParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRecordBody))
	if err != nil {
		s.badRequest(w, fmt.Errorf("读取请求体失败: %v", err))
		return
	}
	var req createMessageRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		s.badRequest(w, fmt.Errorf("请求体不是合法 JSON: %v", err))
		return
	}
	to := strings.TrimSpace(req.ToWxID)
	if to == "" {
		s.badRequest(w, fmt.Errorf("to_wxid 不能为空"))
		return
	}
	if req.MsgType <= 0 {
		s.badRequest(w, fmt.Errorf("msg_type 必须大于 0"))
		return
	}
	self := strings.TrimSpace(s.cfg.WxID)
	if self == "" {
		s.internalError(w, "记录消息失败", fmt.Errorf("服务配置缺少 wxid，无法确定发送人"))
		return
	}

	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" && strings.HasSuffix(to, "@chatroom") {
		roomID = to
	}
	createTime := req.CreateTime
	if createTime <= 0 {
		createTime = time.Now().Unix()
	}

	rec := &store.MessageRecord{
		NewMsgID:   strings.TrimSpace(req.NewMsgID),
		RoomID:     roomID,
		CreateTime: createTime,
		MsgType:    req.MsgType,
		FromWxID:   self,
		ToWxID:     to,
		Content:    req.Content,
		RawJSON:    req.RawJSON,
	}
	inserted, err := s.st.InsertMessage(r.Context(), owner, rec)
	if err != nil {
		s.internalError(w, "记录消息失败", err)
		return
	}
	writeSuccess(w, map[string]any{"inserted": inserted, "new_msg_id": rec.NewMsgID})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	owner, err := s.ownerParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	state, err := s.st.GetSyncState(r.Context(), owner)
	if err != nil {
		s.internalError(w, "查询同步状态失败", err)
		return
	}
	counts, err := s.st.Counts(r.Context(), owner)
	if err != nil {
		s.internalError(w, "统计数据失败", err)
		return
	}
	writeSuccess(w, syncStatusData{
		AppID:         owner,
		WxID:          s.cfg.WxID,
		Status:        state.Status,
		LastSyncTime:  formatDBTime(state.LastSyncAt),
		LastError:     state.LastError,
		ContactsCount: counts.Contacts,
		MessagesCount: counts.Messages,
		RoomsCount:    counts.Rooms,
		DBDriver:      s.cfg.DBKind(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCallbackBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidParams, "读取回调请求体失败: "+err.Error())
		return
	}
	message, err := s.cb.HandlePayload(r.Context(), body)
	if err != nil {
		if errors.Is(err, callback.ErrInvalidPayload) {
			writeError(w, http.StatusBadRequest, CodeInvalidParams, err.Error())
			return
		}
		s.internalError(w, "处理回调失败", err)
		return
	}
	writeMessage(w, message)
}

func (s *Server) badRequest(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadRequest, CodeInvalidParams, err.Error())
}

func (s *Server) internalError(w http.ResponseWriter, action string, err error) {
	s.logger.Printf("%s: %v", action, err)
	writeError(w, http.StatusInternalServerError, CodeInternalError, action+": "+err.Error())
}

// labelNotFoundError 表示标签名在本地标签表中不存在（客户端参数错误）。
type labelNotFoundError struct{ Ref string }

func (e *labelNotFoundError) Error() string { return "标签 " + e.Ref + " 不存在，请先确认标签名称" }

// resolveLabelIDs 把标签名或标签 ID 解析为标签 ID 列表：纯数字按 ID 处理，否则按名称匹配（忽略大小写）。
func (s *Server) resolveLabelIDs(ctx context.Context, owner string, refs []string) ([]int64, error) {
	labels, err := s.st.ListLabels(ctx, owner)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]int64, len(labels))
	for _, l := range labels {
		byName[strings.ToLower(strings.TrimSpace(l.LabelName))] = l.LabelID
	}
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
			ids = append(ids, id)
			continue
		}
		id, ok := byName[strings.ToLower(ref)]
		if !ok {
			return nil, &labelNotFoundError{Ref: ref}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Server) labelDict(r *http.Request, owner string) (map[int64]string, error) {
	labels, err := s.st.ListLabels(r.Context(), owner)
	if err != nil {
		return nil, err
	}
	dict := make(map[int64]string, len(labels))
	for _, l := range labels {
		dict[l.LabelID] = l.LabelName
	}
	return dict, nil
}

func labelIDsAndNames(raw string, dict map[int64]string) ([]int64, []string) {
	ids := make([]int64, 0)
	names := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
		if name, ok := dict[id]; ok && name != "" {
			names = append(names, name)
		} else {
			names = append(names, part)
		}
	}
	return ids, names
}

// conversationIDOf 推导会话 ID：群消息用 R:<roomid>；私聊用 S:<对端 wxid>，
// 由 from_wxid 是否等于本账号 wxid 判断是自己发出还是收到。
func conversationIDOf(m store.Message, selfWxID string) string {
	if m.RoomID != "" {
		return "R:" + m.RoomID
	}
	selfWxID = strings.TrimSpace(selfWxID)
	if selfWxID != "" && m.FromWxID == selfWxID {
		return "S:" + m.ToWxID
	}
	return "S:" + m.FromWxID
}

func contactTypeName(t int) string {
	switch t {
	case store.TypeFriend:
		return "friend"
	case store.TypeChatroom:
		return "chatroom"
	case store.TypeGh:
		return "gh"
	case store.TypeWework:
		return "wework"
	default:
		return "unknown"
	}
}

func messageTypeName(msgType int) string {
	switch msgType {
	case 1:
		return "text"
	case 3:
		return "image"
	case 34:
		return "voice"
	case 43:
		return "video"
	case 42:
		return "card"
	case 47:
		return "emoji"
	case 48:
		return "location"
	case 49:
		return "app"
	case 10000:
		return "system"
	case 10002:
		return "sysmsg"
	default:
		return "other"
	}
}

func formatDBTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.DateTime, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

func formatUnixTime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}
