// 数据同步服务集成：登录成功（或 login status 检测到在线）后把主 CLI 的
// token/appId/baseURL/wxid 写入 wekeapi-msg-sync.json 并确保服务运行；
// 已运行且凭证无变化时保持不动（不重复启动），仅未运行或凭证变化时才启动/重启；
// 首次创建配置时交互终端询问数据库类型。
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wekeapi-cli/internal/msgservice"
	"wekeapi-cli/internal/runner"
)

// syncMsgService 同步服务配置并确保服务运行：
// 服务已在运行且凭证无变化时不重启；需要重新全量同步请用 wekeapi data sync。
func syncMsgService(ctx context.Context, opts *runner.Options) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var missing []string
	if strings.TrimSpace(cfg.Token) == "" {
		missing = append(missing, "token")
	}
	if strings.TrimSpace(cfg.AppID) == "" {
		missing = append(missing, "appId")
	}
	if strings.TrimSpace(cfg.WxID) == "" {
		missing = append(missing, "wxid")
	}
	if len(missing) > 0 {
		return fmt.Errorf("数据同步服务未启动：配置缺少 %s（请先登录成功，或运行 wekeapi init 补全）",
			strings.Join(missing, "/"))
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	layout, err := msgservice.ResolveLayout("", cfg.MsgSyncDir)
	if err != nil {
		return err
	}
	logf := func(format string, args ...any) {
		fmt.Fprintf(opts.Stderr, "[msg-sync] "+format+"\n", args...)
	}
	// 首次创建服务配置时：交互终端询问数据库类型（默认 SQLite），
	// 非交互场景（skill/脚本）不提示，由 Ensure 按默认 SQLite 处理。
	var askDB func() (*msgservice.ConfigureOptions, error)
	if stdinIsTerminal() {
		askDB = func() (*msgservice.ConfigureOptions, error) {
			return promptMsgSyncDatabase(opts.Stderr, bufio.NewReader(os.Stdin))
		}
	}
	st, err := msgservice.Ensure(ctx, layout, msgservice.EnsureOptions{
		Settings: msgservice.Settings{
			Token:   cfg.Token,
			AppID:   cfg.AppID,
			BaseURL: baseURL,
			WxID:    cfg.WxID,
		},
		AskDB: askDB,
		Logf:  logf,
	})
	if err != nil {
		return err
	}
	switch st.Action {
	case "unchanged":
		fmt.Fprintf(opts.Stderr, "数据同步服务运行中：端口 %d，数据目录 %s\n",
			st.Port, filepath.Dir(st.ConfigPath))
	default:
		fmt.Fprintf(opts.Stderr, "数据同步服务已%s：pid=%d，端口 %d，配置 %s\n",
			actionText(st.Action), st.PID, st.Port, st.ConfigPath)
	}
	return nil
}

func actionText(action string) string {
	switch action {
	case "restarted":
		return "重启"
	case "started":
		return "启动"
	default:
		return "更新"
	}
}

// promptMsgSyncDatabase 首次创建数据同步服务配置时询问数据库类型（默认 SQLite）。
// 提示一律写 stderr，保持 stdout 的 JSON 契约；in 为用户输入（交互终端为 stdin）。
func promptMsgSyncDatabase(w io.Writer, in *bufio.Reader) (*msgservice.ConfigureOptions, error) {
	fmt.Fprint(w, `未检测到数据同步服务配置，请选择数据库类型：
  1) SQLite（默认，本地文件 wekeapi_msg_sync.db，无需额外部署）
  2) MySQL（使用已有 MySQL 实例，需提供连接串）
请输入 1 或 2（回车使用默认 1）: `)
	line, _ := in.ReadString('\n')
	switch choice := strings.TrimSpace(line); choice {
	case "", "1":
		fmt.Fprintln(w, "已选择 SQLite")
		return &msgservice.ConfigureOptions{DB: "sqlite"}, nil
	case "2":
		fmt.Fprint(w, "请输入 MySQL 连接串（如 mysql://user:pass@127.0.0.1:3306/dbname，库不存在时服务启动会自动创建）: ")
		dsn, _ := in.ReadString('\n')
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			return nil, fmt.Errorf("未输入 MySQL 连接串，已跳过数据同步服务初始化；可稍后用 wekeapi data config --db mysql --db-dsn <连接串> 配置")
		}
		return &msgservice.ConfigureOptions{DB: "mysql", DSN: dsn}, nil
	default:
		return nil, fmt.Errorf("无效的数据库类型选择 %q：请输入 1（SQLite）或 2（MySQL），也可用 wekeapi data config 手动配置", choice)
	}
}
