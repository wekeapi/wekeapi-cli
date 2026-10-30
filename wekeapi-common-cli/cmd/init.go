// init 命令与配置文件：将 AccessToken/appId 保存到 ~/.wekeapi/config.json，
// 供后续命令免传 --token/--app-id（显式 flag 优先于配置文件）。
// 兼容旧版单文件配置 ~/.wekeapi.json：新路径不存在时自动回退读取（只读），
// 首次写入（init 或登录成功）后即迁移到新位置；旧文件保留不删除。
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"wekeapi-cli/internal/env"
	"wekeapi-cli/internal/runner"
)

// cliConfig 配置文件结构。
type cliConfig struct {
	Token      string `json:"token"`
	AppID      string `json:"appId"`
	BaseURL    string `json:"baseURL,omitempty"`
	WxID       string `json:"wxid,omitempty"`       // 当前登录账号 wxid，登录成功后自动记录
	MsgSyncDir string `json:"msgSyncDir,omitempty"` // wekeapi-msg-sync 服务目录（可选，默认自动查找）
}

// dataDir 返回数据目录（~/.wekeapi），配置文件存放于此。
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, ".wekeapi"), nil
}

// configPath 返回配置文件路径（~/.wekeapi/config.json）。
func configPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// legacyConfigPath 返回旧版配置文件路径（~/.wekeapi.json，只读兼容）。
func legacyConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, ".wekeapi.json"), nil
}

// loadConfig 读取配置文件；新路径不存在时回退旧版 ~/.wekeapi.json（只读）；均不存在时返回零值。
func loadConfig() (cliConfig, error) {
	path, err := configPath()
	if err != nil {
		return cliConfig{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return cliConfig{}, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
		// 回退旧版单文件配置（只读，首次保存时自然迁移到新路径）
		legacy, lerr := legacyConfigPath()
		if lerr != nil {
			return cliConfig{}, nil
		}
		b, err = os.ReadFile(legacy)
		if err != nil {
			if os.IsNotExist(err) {
				return cliConfig{}, nil
			}
			return cliConfig{}, fmt.Errorf("读取配置文件 %s 失败: %w", legacy, err)
		}
		path = legacy
	}
	var cfg cliConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cliConfig{}, fmt.Errorf("配置文件 %s 不是合法 JSON（请修复，或运行 wekeapi init 覆盖）: %w", path, err)
	}
	return cfg, nil
}

// saveConfig 写入配置文件（目录 0700、文件 0600，token 为敏感信息）。
func saveConfig(cfg cliConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录 %s 失败: %w", filepath.Dir(path), err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("写入配置文件 %s 失败: %w", path, err)
	}
	return nil
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// stdinIsTerminal 判断 stdin 是否为交互终端（管道/NUL/重定向均为 false，跳过向导提示）。
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// newInitCmd 构建 init 命令：交互终端下依次提示输入 AccessToken、确认 baseUrl；
// 非交互（脚本/skill 自动化）不提示，全部通过 flag 传入。提示一律走 stderr，stdout 保持 JSON 契约。
func newInitCmd(opts *runner.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "初始化配置：保存 AccessToken、appId 与 base-url 到 ~/.wekeapi/config.json",
		Long: fmt.Sprintf(`初始化配置信息，保存到 ~/.wekeapi/config.json；之后所有命令自动读取，可免传 --token/--app-id/--base-url（显式 flag 优先）。

交互终端直接运行 wekeapi init 会依次提示（提示走 stderr）：
  1) 输入 AccessToken（到官网 %s 申请；已有配置时直接回车保留）；
  2) 确认 API baseUrl（回车使用当前值或默认值，或输入新地址修改）。
脚本/skill 自动化场景（stdin 非交互终端）不会提示；--base-url 省略时使用编译内置默认值 %s。

--token 可选：显式传入时跳过输入提示（AccessToken 请到官网 %s 申请）。
--app-id 可选：首次登录扫码时可为空；省略时保留配置中已有的 appId，显式传 --app-id=（等号空值）可清空
（不要写 --app-id ""：Windows PowerShell 5.1 会丢弃空字符串参数，导致 --app-id 吞掉后一个 flag）。
--base-url 可选：API 基础地址，默认 %s；显式传入时跳过确认提示，省略时保留配置中已有的值，显式传 --base-url= 可恢复默认。最终生效的 baseUrl 一律记录到配置文件。
扫码登录成功后（login check 返回 status=2），本次使用的 appId 会自动保存到配置文件。
兼容旧版单文件配置 ~/.wekeapi.json：其存在时自动读取，首次保存后迁移到 ~/.wekeapi/config.json（旧文件保留不删除）。

--msg-sync-dir 可选：指定 wekeapi-msg-sync 服务目录（含 wekeapi-msg-sync.json 与可执行文件）。
省略时自动从当前目录/可执行文件目录向上查找 wekeapi-msg-sync，或使用环境变量 WEKEAPI_MSG_SYNC_HOME。
登录成功后会自动把 token/appId/wxid/baseURL 写入该目录的 wekeapi-msg-sync.json，并确保服务运行（已运行且凭证无变化时不重启）。`, env.SiteURL, env.SiteURL, defaultBaseURL, defaultBaseURL),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pf := cmd.Root().PersistentFlags()
			cfg, err := loadConfig()
			if err != nil {
				fmt.Fprintf(opts.Stderr, "error: %v\n", err)
				return &runner.ExitError{Code: 2, Err: err}
			}
			interactive := stdinIsTerminal()
			var stdin *bufio.Reader
			readLine := func() string {
				if stdin == nil {
					stdin = bufio.NewReader(os.Stdin)
				}
				line, err := stdin.ReadString('\n')
				if err != nil && line == "" {
					return ""
				}
				return strings.TrimSpace(line)
			}

			// 1) AccessToken：显式 flag > 交互提示（回车保留已保存值）> 配置已有 > 报错引导
			if !pf.Changed("token") && opts.Token == "" && interactive {
				if cfg.Token != "" {
					fmt.Fprintf(opts.Stderr, "请输入 AccessToken（回车保留已保存的 %s；到官网 %s 申请）: ", maskToken(cfg.Token), env.SiteURL)
					if s := readLine(); s != "" {
						opts.Token = s
					}
				} else {
					fmt.Fprintf(opts.Stderr, "请输入 AccessToken（到官网 %s 申请）: ", env.SiteURL)
					opts.Token = readLine()
				}
				if opts.Token == "" && cfg.Token != "" {
					opts.Token = cfg.Token // 直接回车：保留已保存的 token
				}
			}
			if opts.Token == "" {
				err := fmt.Errorf("init 需要提供 --token <AccessToken>（AccessToken 请到官网 %s 申请；或在交互终端直接运行 wekeapi init 按提示输入）", env.SiteURL)
				fmt.Fprintf(opts.Stderr, "error: %v\n", err)
				return &runner.ExitError{Code: 2, Err: err}
			}

			// 2) baseUrl：显式 flag > 交互确认（回车保留当前/默认）> 配置已有 > 编译内置默认值
			if pf.Changed("base-url") {
				cfg.BaseURL = opts.BaseURL
			} else if interactive {
				current := cfg.BaseURL
				if current == "" {
					current = defaultBaseURL
				}
				if current == defaultBaseURL {
					fmt.Fprintf(opts.Stderr, "API baseUrl（回车使用默认 %s，或输入新地址修改）: ", defaultBaseURL)
				} else {
					fmt.Fprintf(opts.Stderr, "API baseUrl（回车保留 %s；默认 %s，输入新地址可修改）: ", current, defaultBaseURL)
				}
				if s := readLine(); s != "" {
					cfg.BaseURL = s
				}
			}

			cfg.Token = opts.Token
			if pf.Changed("app-id") {
				cfg.AppID = opts.AppID
			}
			if cmd.Flags().Changed("msg-sync-dir") {
				dir, _ := cmd.Flags().GetString("msg-sync-dir")
				cfg.MsgSyncDir = strings.TrimSpace(dir)
				if cfg.MsgSyncDir != "" {
					if abs, aerr := filepath.Abs(cfg.MsgSyncDir); aerr == nil {
						cfg.MsgSyncDir = abs
					}
				}
			}
			// 最终生效的 baseUrl 一律落盘（显式传入 > 配置已有 > 默认值），
			// 配置中不存空串，便于直接从配置确认实际使用的 API 地址。
			if cfg.BaseURL == "" {
				cfg.BaseURL = defaultBaseURL
			}
			if err := saveConfig(cfg); err != nil {
				fmt.Fprintf(opts.Stderr, "error: %v\n", err)
				return &runner.ExitError{Code: 2, Err: err}
			}
			path, _ := configPath()
			fmt.Fprintf(opts.Stderr, "API baseUrl：默认 %s；当前生效 %s（已记录到配置）。修改：重新运行 wekeapi init 按提示输入，或传 --base-url <URL>（恢复默认传 --base-url \"\"）\n", defaultBaseURL, cfg.BaseURL)
			return writeLocalResult(opts, map[string]any{
				"configPath":     path,
				"token":          maskToken(cfg.Token),
				"appId":          cfg.AppID,
				"baseURL":        cfg.BaseURL,
				"baseURLDefault": defaultBaseURL,
				"msgSyncDir":     cfg.MsgSyncDir,
			})
		},
	}
	return cmd
}

// notifyLoginResult 在检测登录状态返回 status=2（登录成功）时：
// 1) 把本次登录使用的 appId 保存到配置文件（关键凭证，先落盘、不依赖后续请求）；
// 2) 自动调用一次在线状态检测，统一采集个人信息并记录 wxid；
// 3) 把 token/appId/baseURL/wxid 同步到 wekeapi-msg-sync.json，并确保数据同步服务运行
//    （已运行且凭证无变化时保持不动，不重复启动）。
// 注意：appId 只在登录成功后落盘；获取二维码阶段不保存，避免未登录成功的设备残留配置。
func notifyLoginResult(ctx context.Context, opts *runner.Options, data map[string]any) error {
	if anyInt64(data["status"]) != 2 {
		return nil
	}
	if opts.AppID == "" {
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.AppID != opts.AppID {
		cfg.AppID = opts.AppID
		if err := saveConfig(cfg); err != nil {
			return err
		}
		path, _ := configPath()
		fmt.Fprintf(opts.Stderr, "登录成功：appId=%s（已保存到 %s）\n", opts.AppID, path)
	}
	if err := refreshAccountInfo(ctx, opts); err != nil {
		return err
	}
	// 登录成功后：把 token/appId/baseURL/wxid 写入 wekeapi-msg-sync.json，
	// 并确保数据同步服务运行：已运行且凭证无变化时保持不动，仅未运行/凭证变化时才启动。
	return syncMsgService(ctx, opts)
}
