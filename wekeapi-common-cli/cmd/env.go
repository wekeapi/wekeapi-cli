// env 命令：查看编译期内置的环境信息，
// 供 skill/调用方查询本二进制的默认 baseUrl、官网地址与各平台代理工具下载地址。
package cmd

import (
	"github.com/spf13/cobra"

	"wekeapi-cli/internal/env"
	"wekeapi-cli/internal/runner"
)

func newEnvCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "查看编译时内置的环境信息（环境名/baseUrl/官网/代理工具下载地址）",
		Long: `查看当前二进制编译时内置的环境信息，stdout 输出 JSON：
  env            环境名（默认 prod）
  baseURL        API 基础地址默认值
  siteURL        官网地址（申请 AccessToken 入口）
  proxyDownloads 各平台本地代理工具下载地址（Linux 含一键安装命令）
无需 --token/--app-id；skill 场景应通过本命令查询地址，不要硬编码。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeLocalResult(opts, map[string]any{
				"env":            env.Name,
				"baseURL":        env.BaseURL,
				"siteURL":        env.SiteURL,
				"proxyDownloads": env.ProxyDownloads,
			})
		},
	}
}
