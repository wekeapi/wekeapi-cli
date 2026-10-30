// Package cmd 定义 wekeapi CLI 的全部命令。
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/env"
	"wekeapi-cli/internal/runner"
)

// defaultBaseURL 编译期由环境（internal/env，build tags 区分测试/线上）决定。
var defaultBaseURL = env.BaseURL

var opts = &runner.Options{
	Stdout: os.Stdout,
	Stderr: os.Stderr,
}

var rootCmd = &cobra.Command{
	Use:   "wekeapi",
	Short: "wekeapi 微信协议 API 命令行工具",
	Long: `wekeapi 是 wekeapi（微信协议 API）的命令行客户端。

所有接口均为 POST JSON，通过 --token 提供 AccessToken（X-Access-Token）。
stdout 仅输出 API 返回的 JSON；错误信息输出到 stderr。
联系人/消息/群聊等业务数据由 wekeapi-msg-sync 服务统一存储，
用 data 命令查询（data contacts/messages/rooms/status），不再维护本地数据库。
退出码：0=成功，1=API 业务错误（code!=0），2=本地错误（参数/网络/文件等）。`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&opts.Token, "token", "", "访问凭证 AccessToken（必填；可运行 wekeapi init 保存后免传）")
	pf.StringVar(&opts.AppID, "app-id", "", "设备 appId（登录接口之外的绝大多数接口必填；可运行 wekeapi init 保存后免传；显式清空传 --app-id=）")
	pf.StringVar(&opts.BaseURL, "base-url", defaultBaseURL, "API 基础地址")
	pf.DurationVar(&opts.Timeout, "timeout", 60*time.Second, "HTTP 超时时间（大文件上传可调大）")
	pf.BoolVar(&opts.Pretty, "pretty", false, "格式化（缩进）输出 JSON")
	pf.StringVar(&opts.Data, "data", "", "直接指定请求 JSON body：传 JSON 字符串或 @文件路径（Windows 建议 @文件，引号易被 shell 剥离）；与命令自带参数合并，同名字段以命令参数为准；传完整接口响应（含 code/data）时自动取其中的 data 作 body")
	// 全局可用的服务目录参数：只有需要读写 wekeapi-msg-sync 的命令（data/login/message 等）实际使用它，
	// 放在全局以避免在脚本里对每条命令都要判断能否传。
	pf.String("msg-sync-dir", "", "wekeapi-msg-sync 服务目录（含 wekeapi-msg-sync.json）；省略时按环境变量/配置/默认目录自动查找")

	// 命令执行前合并配置文件中的凭证（显式 flag 优先于 ~/.wekeapi/config.json）。
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return &runner.ExitError{Code: 2, Err: err}
		}
		if !pf.Changed("token") && cfg.Token != "" {
			opts.Token = cfg.Token
		}
		// 显式 --app-id（含 --app-id= 的空值）优先于配置：AppIDSet 记录「用户显式给过」，
		// 值为空时表示按新设备处理（配置不再参与覆盖）。
		opts.AppIDSet = pf.Changed("app-id")
		if opts.AppIDSet && strings.HasPrefix(opts.AppID, "-") {
			// 典型事故：shell 丢弃了空字符串参数（如 Windows PowerShell 5.1 的 --app-id ""），
			// 使 --app-id 把紧随其后的 flag 当成值吞掉（如 --out、--pretty）。
			return fmt.Errorf("--app-id 的值 %q 以 - 开头，疑似 shell 丢弃空参数后把后一个 flag 当成了 appId；如需显式清空 appId（新设备）请用 --app-id= 或 login qr --new-device", opts.AppID)
		}
		if !opts.AppIDSet && cfg.AppID != "" {
			opts.AppID = cfg.AppID
		}
		if !pf.Changed("base-url") && cfg.BaseURL != "" {
			opts.BaseURL = cfg.BaseURL
		}
		// 当前账号 wxid（转发类命令补全 oriMsgFrom 用），来自配置；无对应 flag。
		opts.WxID = cfg.WxID
		return nil
	}

	rootCmd.AddCommand(
		newInitCmd(opts),
		newEnvCmd(opts),
		runner.NewGroup("login", "登录模块", loginSpecs, opts),
		runner.NewGroup("contact", "通讯录模块", contactSpecs, opts),
		runner.NewGroup("chatroom", "群聊模块", chatroomSpecs, opts),
		runner.NewGroup("label", "标签模块", labelSpecs, opts),
		runner.NewGroup("message", "消息模块", messageSpecs, opts),
		newSnsGroup(opts),
		newProxyGroup(opts),
		newDataCmd(opts),
	)
}

// Execute 执行根命令，返回进程退出码。
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *runner.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		fmt.Fprintf(opts.Stderr, "error: %v\n", err)
		return 2
	}
	return 0
}
