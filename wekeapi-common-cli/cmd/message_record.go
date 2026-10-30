// message_record.go 负责「自己发送的消息」落库：发送类命令成功后把消息增量写入
// wekeapi-msg-sync，使 data messages 能同时查到收到与发出的消息。
package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/msgservice"
	"wekeapi-cli/internal/runner"
)

// sentMsgTypes 发送类命令对应的微信消息类型（与 wekeapi-msg-sync 的 type 映射一致）。
var sentMsgTypes = map[string]int{
	"send-text":         1,  // 文本
	"send-image":        3,  // 图片
	"send-voice":        34, // 语音
	"send-video":        43, // 视频
	"send-card":         42, // 名片
	"send-emoji":        47, // 表情
	"send-link":         49, // 链接
	"send-file":         49, // 文件
	"send-miniapp":      49, // 小程序
	"send-appmsg":       49, // AppMsg
	"forward-image":     3,
	"forward-image-msg": 3,
	"forward-video":     43,
	"forward-video-msg": 43,
	"forward-file":      49,
	"forward-file-msg":  49,
	"forward-link":      49,
	"forward-link-msg":  49,
	"forward-miniapp":   49,
}

// init 给发送/转发类命令挂上 PostRun：发送成功后把消息写入数据同步服务。
// 与服务已通过回调记录的同一消息不会重复入库（按 wx_id + new_msg_id 幂等）。
func init() {
	for i := range messageSpecs {
		if msgType, ok := sentMsgTypes[messageSpecs[i].Use]; ok {
			messageSpecs[i].PostRun = recordSentMessage(msgType)
		}
	}
}

// recordSentMessage 生成 PostRun：from 由服务端取配置的 wxid，to/newMsgId/createTime
// 取发送响应（缺失时回落到请求体），msg_type 按命令映射。
func recordSentMessage(msgType int) runner.PostRunFunc {
	return func(cmd *cobra.Command, opts *runner.Options, body, data map[string]any) error {
		to := anyString(data["toWxid"])
		if to == "" {
			to = anyString(body["toWxid"])
		}
		if to == "" || opts.WxID == "" {
			// 无接收人或本地无 wxid（未登录）时无法还原会话，跳过。
			return nil
		}
		msg := msgservice.SentMessage{
			NewMsgID:   anyString(data["newMsgId"]),
			ToWxID:     to,
			MsgType:    msgType,
			CreateTime: anyInt64(data["createTime"]),
			Content:    sentContent(body),
			RawJSON:    sentRawJSON(data),
		}
		return recordSent(cmd, msg)
	}
}

// recordSent 交由数据同步服务落库；服务未配置/未运行时静默跳过（不影响发送结果）。
func recordSent(cmd *cobra.Command, msg msgservice.SentMessage) error {
	layout, err := resolveMsgLayout(cmd)
	if err != nil {
		return nil
	}
	return msgservice.RecordSent(cmd.Context(), layout, msg)
}

// sentContent 提取可检索内容：文本取 content，链接取标题+地址，文件取文件名，名片取昵称；
// 图片/语音/视频等媒体类留空，避免把 base64 文件数据写进数据库。
func sentContent(body map[string]any) string {
	if v := anyString(body["content"]); v != "" {
		return v
	}
	if title := anyString(body["title"]); title != "" {
		if url := anyString(body["linkUrl"]); url != "" {
			return title + " " + url
		}
		return title
	}
	for _, key := range []string{"fileName", "cardNickname"} {
		if v := anyString(body[key]); v != "" {
			return v
		}
	}
	return ""
}

// sentRawJSON 保留发送响应（newMsgId/fileId 等便于排查的字段），
// 超长响应直接丢弃，避免异常数据撑大数据库。
func sentRawJSON(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	const maxRaw = 64 << 10
	if len(b) > maxRaw {
		return ""
	}
	return string(b)
}
