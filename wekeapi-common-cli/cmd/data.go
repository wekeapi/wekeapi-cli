// data 命令组：查询 wekeapi-msg-sync 的数据（联系人/消息/群聊/同步状态），
// 以及启动、停止、重启数据同步服务。主 CLI 不再维护本地数据库。
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/msgservice"
	"wekeapi-cli/internal/runner"
)

// newDataCmd 构建 data 模块命令组。
func newDataCmd(opts *runner.Options) *cobra.Command {
	g := &cobra.Command{
		Use:   "data",
		Short: "数据查询与同步服务管理（wekeapi-msg-sync）",
		Long: `数据查询与同步服务管理。

联系人、消息、群聊等数据由 wekeapi-msg-sync 服务统一接收回调、同步并存储；
本 CLI 不再维护本地数据库，查询通过服务的 REST API 完成。

登录成功（login check 返回 status=2）时，CLI 会自动把 token/appId/baseURL/wxid
写入服务目录下的 wekeapi-msg-sync.json，并确保 wekeapi-msg-sync 运行：
已运行且凭证无变化时不重启，仅在未运行或凭证变化时启动/重启。
需要重新全量同步时用 "data sync"。
数据库（默认 SQLite）与服务端口（默认 3006）用 "data config" 修改，
改后执行 "data sync" 重启生效。

服务目录（配置、日志、PID、SQLite 均在其中）查找顺序：--msg-sync-dir >
环境变量 WEKEAPI_MSG_SYNC_HOME > ~/.wekeapi/config.json 的 msgSyncDir >
默认 ~/.wekeapi/msg-sync。`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	g.AddCommand(
		newDataContactsCmd(opts),
		newDataMessagesCmd(opts),
		newDataRoomsCmd(opts),
		newDataStatusCmd(opts),
		newDataConfigCmd(opts),
		newDataStartCmd(opts),
		newDataStopCmd(opts),
		newDataSyncCmd(opts),
	)
	return g
}

func newDataContactsCmd(opts *runner.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contacts",
		Short: "查询联系人（好友/群/公众号/企微好友）",
		Long: `查询数据同步服务中的联系人。

用于把微信号/昵称/备注等关键字解析成 wxid：--keyword 会同时匹配
昵称、备注、别名与 wxid（contact_id）。查不到再调用远端通讯录接口。

--label 按标签查找联系人：传标签名称或标签 ID，多个用逗号分隔表示同时具备。
--type 可选：friend/friends、chatroom/rooms、gh/ghs、wework、all（默认全部）。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			typeName, _ := cmd.Flags().GetString("type")
			keyword, _ := cmd.Flags().GetString("keyword")
			label, _ := cmd.Flags().GetString("label")
			page, _ := cmd.Flags().GetInt("page")
			limit, _ := cmd.Flags().GetInt("limit")

			q := url.Values{}
			if v := strings.TrimSpace(typeName); v != "" {
				q.Set("type", v)
			}
			if v := strings.TrimSpace(keyword); v != "" {
				q.Set("keyword", v)
			}
			if v := strings.TrimSpace(label); v != "" {
				q.Set("label", v)
			}
			addPageParams(q, page, limit)
			return runDataQuery(cmd, opts, "/api/contacts", q)
		},
	}
	cmd.Flags().String("type", "", "联系人类型：friend/chatroom/gh/wework/all")
	cmd.Flags().String("keyword", "", "关键字（昵称/备注/别名/wxid）")
	cmd.Flags().String("label", "", "标签过滤：标签名或标签 ID，多个用逗号分隔（同时具备）")
	cmd.Flags().Int("page", 0, "页码（默认 1）")
	cmd.Flags().Int("limit", 0, "每页数量（默认 20，最大 100）")
	return cmd
}

func newDataMessagesCmd(opts *runner.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "messages",
		Short: "查询消息记录",
		Long: `查询数据同步服务中的消息（按时间倒序）。

--conversation-id：私聊 S:<wxid>，群聊 R:<roomid>；
--start-time/--end-time 支持 RFC3339、2006-01-02 15:04:05、2006-01-02 或 Unix 秒。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			conversation, _ := cmd.Flags().GetString("conversation-id")
			keyword, _ := cmd.Flags().GetString("keyword")
			startTime, _ := cmd.Flags().GetString("start-time")
			endTime, _ := cmd.Flags().GetString("end-time")
			page, _ := cmd.Flags().GetInt("page")
			limit, _ := cmd.Flags().GetInt("limit")

			q := url.Values{}
			if v := strings.TrimSpace(conversation); v != "" {
				q.Set("conversation_id", v)
			}
			if v := strings.TrimSpace(keyword); v != "" {
				q.Set("keyword", v)
			}
			if v := strings.TrimSpace(startTime); v != "" {
				q.Set("start_time", v)
			}
			if v := strings.TrimSpace(endTime); v != "" {
				q.Set("end_time", v)
			}
			addPageParams(q, page, limit)
			return runDataQuery(cmd, opts, "/api/messages", q)
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 ID：私聊 S:<wxid>，群聊 R:<roomid>")
	cmd.Flags().String("keyword", "", "消息内容关键字")
	cmd.Flags().String("start-time", "", "开始时间（RFC3339/2006-01-02 15:04:05/2006-01-02/Unix 秒）")
	cmd.Flags().String("end-time", "", "结束时间（同上）")
	cmd.Flags().Int("page", 0, "页码（默认 1）")
	cmd.Flags().Int("limit", 0, "每页数量（默认 20，最大 100）")
	return cmd
}

func newDataRoomsCmd(opts *runner.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "rooms",
		Short:         "查询群聊",
		Long:          `查询数据同步服务中的群聊（支持 --keyword 按群名/备注/群 ID 过滤）。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword, _ := cmd.Flags().GetString("keyword")
			page, _ := cmd.Flags().GetInt("page")
			limit, _ := cmd.Flags().GetInt("limit")

			q := url.Values{}
			if v := strings.TrimSpace(keyword); v != "" {
				q.Set("keyword", v)
			}
			addPageParams(q, page, limit)
			return runDataQuery(cmd, opts, "/api/rooms", q)
		},
	}
	cmd.Flags().String("keyword", "", "关键字（群名/备注/群 ID）")
	cmd.Flags().Int("page", 0, "页码（默认 1）")
	cmd.Flags().Int("limit", 0, "每页数量（默认 20，最大 100）")
	return cmd
}

func newDataStatusCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "查看数据同步状态与数据量",
		Long:          `查看数据同步服务的同步状态（idle/syncing/error）、最近同步时间与联系人/群聊/消息数量。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataQuery(cmd, opts, "/api/sync-status", nil)
		},
	}
}

func newDataStartCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:           "start",
		Short:         "启动数据同步服务（已运行则保持不动）",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := resolveMsgLayout(cmd)
			if err != nil {
				return failData(opts, err)
			}
			st, err := msgservice.Start(cmd.Context(), layout)
			if err != nil {
				return failData(opts, err)
			}
			return writeLocalResult(opts, st)
		},
	}
}

func newDataStopCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:           "stop",
		Short:         "停止数据同步服务",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := resolveMsgLayout(cmd)
			if err != nil {
				return failData(opts, err)
			}
			st, err := msgservice.Stop(cmd.Context(), layout)
			if err != nil {
				return failData(opts, err)
			}
			return writeLocalResult(opts, st)
		},
	}
}

func newDataSyncCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:           "sync",
		Aliases:       []string{"restart"},
		Short:         "重启数据同步服务（触发一次全量同步）",
		Long:          `重启数据同步服务：已运行时先关闭再启动；服务启动后会立即执行一次全量同步。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := resolveMsgLayout(cmd)
			if err != nil {
				return failData(opts, err)
			}
			st, err := msgservice.Restart(cmd.Context(), layout)
			if err != nil {
				return failData(opts, err)
			}
			return writeLocalResult(opts, st)
		},
	}
}

// resolveMsgLayout 解析服务目录：--msg-sync-dir > 环境变量 > CLI 配置 msgSyncDir > 自动查找。
// 命令未提供 --msg-sync-dir（如 message send-*）时按后三者查找。
func resolveMsgLayout(cmd *cobra.Command) (*msgservice.Layout, error) {
	flagDir := ""
	if cmd != nil {
		if fl := cmd.Flags().Lookup("msg-sync-dir"); fl != nil {
			flagDir, _ = cmd.Flags().GetString("msg-sync-dir")
		}
	}
	fallback := ""
	if cfg, err := loadConfig(); err == nil {
		fallback = cfg.MsgSyncDir
	}
	return msgservice.ResolveLayout(flagDir, fallback)
}

// msgServiceEnvelope 数据同步服务响应：成功为 {success:true,data:...}，
// 失败为 {success:false,error:"...",code:"INVALID_PARAMS"}。
type msgServiceEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// runDataQuery 调用数据同步服务的 REST API，并统一转换为 CLI 的
// {"code":0,"message":"OK","data":...} 输出契约。
func runDataQuery(cmd *cobra.Command, opts *runner.Options, path string, q url.Values) error {
	layout, err := resolveMsgLayout(cmd)
	if err != nil {
		return failData(opts, err)
	}
	cfg, err := layout.Config()
	if err != nil {
		return failData(opts, err)
	}
	cl := msgservice.NewAPIClient(cfg.Port)
	raw, err := cl.Get(cmd.Context(), path, q)

	var env msgServiceEnvelope
	parseErr := json.Unmarshal(raw, &env)
	if err != nil {
		// 服务返回了错误响应（HTTP 4xx/5xx）时优先展示服务端错误信息。
		if parseErr == nil && !env.Success && env.Error != "" {
			return failDataAPI(opts, env.Error)
		}
		fmt.Fprintf(opts.Stderr, "error: %v\n数据同步服务可能未启动：可运行 wekeapi data start", err)
		if path != "/api/sync-status" {
			fmt.Fprint(opts.Stderr, "，或查看 wekeapi data status")
		}
		fmt.Fprintln(opts.Stderr)
		return &runner.ExitError{Code: 2, Err: err}
	}
	if parseErr != nil {
		return failData(opts, fmt.Errorf("解析数据同步服务响应失败: %w", parseErr))
	}
	if !env.Success {
		msg := env.Error
		if msg == "" {
			msg = "数据同步服务返回失败"
		}
		return failDataAPI(opts, msg)
	}
	return writeDataSuccess(opts, env.Data)
}

func writeDataSuccess(opts *runner.Options, data json.RawMessage) error {
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	b := make([]byte, 0, len(data)+32)
	b = append(b, `{"code":0,"message":"OK","data":`...)
	b = append(b, data...)
	b = append(b, '}')
	return writeDataRaw(opts, b)
}

func failDataAPI(opts *runner.Options, message string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"code": 1, "message": message})
	_ = writeDataRaw(opts, bytes.TrimRight(buf.Bytes(), "\n"))
	fmt.Fprintf(opts.Stderr, "api error: %s\n", message)
	return &runner.ExitError{Code: 1, Err: fmt.Errorf("api error: %s", message)}
}

func writeDataRaw(opts *runner.Options, raw []byte) error {
	if opts.Pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			raw = buf.Bytes()
		}
	}
	fmt.Fprintln(opts.Stdout, string(raw))
	return nil
}

func addPageParams(q url.Values, page, limit int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
}

func failData(opts *runner.Options, err error) error {
	fmt.Fprintf(opts.Stderr, "error: %v\n", err)
	return &runner.ExitError{Code: 2, Err: err}
}
