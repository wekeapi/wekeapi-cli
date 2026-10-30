// wekeapi-msg-sync 数据同步服务：接收消息回调、增量同步联系人、存储消息并提供查询 API。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	// 嵌入时区数据库：Windows 无系统 tzdata，MySQL DSN 的 loc=Asia/Shanghai 等时区名
	// 在缺失时会报 unknown time zone，导致服务启动失败。
	_ "time/tzdata"

	"github.com/spf13/cobra"

	"wekeapi-msg-sync/internal/callback"
	"wekeapi-msg-sync/internal/client"
	"wekeapi-msg-sync/internal/config"
	"wekeapi-msg-sync/internal/pidfile"
	"wekeapi-msg-sync/internal/server"
	"wekeapi-msg-sync/internal/store"
	"wekeapi-msg-sync/internal/syncer"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wekeapi-msg-sync",
		Short: "wekeapi 数据同步服务（回调接收 + 增量同步 + HTTP API）",
		Long: `wekeapi-msg-sync 是 wekeapi 的数据同步服务：

- 接收消息回调（AddMsg/ModContact/DelContact），实时写入消息与联系人变更；
- 服务启动（登录成功后由 CLI 拉起）时全量同步一次联系人，之后依赖回调增量更新；
- 数据存储默认 SQLite，可配置 MySQL；
- 提供 REST API 查询联系人、消息、群聊与同步状态。

启动：wekeapi-msg-sync run [--config <配置文件>] [--port <端口>]`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCmd())
	return root
}

func newRunCmd() *cobra.Command {
	var (
		configPath string
		port       int
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "启动数据同步服务",
		Long: `启动数据同步服务。

配置文件 --config 省略时使用默认位置 ~/.wekeapi/msg-sync/wekeapi-msg-sync.json
（登录成功时由主 CLI 自动生成，同目录下还有 PID、日志与 SQLite 数据库）；
--port 省略时使用配置文件中的 port，仍未配置则使用 3006。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runService(configPath, port, cmd.Flags().Changed("port"))
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "配置文件路径（默认：~/.wekeapi/msg-sync/wekeapi-msg-sync.json）")
	cmd.Flags().IntVar(&port, "port", 0, "HTTP 监听端口（优先于配置文件 port，默认 3006）")
	return cmd
}

func runService(configPath string, port int, portSet bool) error {
	if configPath == "" {
		configPath = config.DefaultPath()
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if portSet {
		if port < 1 || port > 65535 {
			return fmt.Errorf("--port 必须在 1-65535 之间，当前为 %d", port)
		}
		cfg.Port = port
	}

	logger := log.New(os.Stderr, "[msg-sync] ", log.LstdFlags)
	logf := func(format string, args ...any) { logger.Printf(format, args...) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Printf("配置：%s（appId=%s，wxid=%s，token=%s）",
		cfg.Path, cfg.AppID, cfg.WxID, config.MaskToken(cfg.Token))
	logger.Printf("数据库：%s", dbDescription(cfg))

	// PID 文件供 CLI（wekeapi data start/stop/sync）定位本实例并重启。
	pidPath := pidfile.PathFor(cfg.Path)
	pidConfig := cfg.Path
	if abs, aerr := filepath.Abs(cfg.Path); aerr == nil {
		pidConfig = abs
	}
	if err := pidfile.Write(pidPath, pidfile.Info{
		PID:       os.Getpid(),
		Port:      cfg.Port,
		Config:    pidConfig,
		StartedAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		logger.Printf("写入 PID 文件失败: %v", err)
	} else {
		defer pidfile.Remove(pidPath)
		logger.Printf("PID 文件：%s", pidPath)
	}
	logger.Printf("同步策略：启动时全量同步联系人，之后依赖回调增量更新")

	st, err := store.Open(ctx, store.Options{DBPath: cfg.ResolvedDBPath(), DB: cfg.DB})
	if err != nil {
		return err
	}
	defer st.Close()

	cl := client.New(cfg.BaseURL, cfg.Token)
	cl.OnSyncRetry = func(attempt int, wait time.Duration) {
		logf("通讯录同步进行中，等待 %s 后重试（第 %d 次）", wait, attempt)
	}

	cb := callback.New(st, cfg.AppID, cfg.WxID, logf)
	// ModContacts 到达后异步补拉详情（昵称/标签/群成员数等）。
	cb.SetDetailFetcher(func(ctx context.Context, contactID string) (*store.ContactDetail, bool) {
		details, err := cl.FetchContactDetails(ctx, cfg.AppID, []string{contactID})
		if err != nil {
			logf("拉取联系人详情失败（%s）: %v", contactID, err)
			return nil, false
		}
		if len(details) == 0 {
			return nil, false
		}
		d := details[0]
		return &store.ContactDetail{
			ContactID:   d.ContactID,
			Nickname:    d.Nickname,
			Alias:       d.Alias,
			Avatar:      d.Avatar,
			Remark:      d.Remark,
			Sex:         d.Sex,
			Signature:   d.Signature,
			Labels:      d.Labels,
			MemberCount: d.MemberCount,
			RawJSON:     d.RawJSON,
		}, true
	})
	sy := syncer.New(cfg, cl, st, logf)
	srv := server.New(cfg, st, cb, logger)

	// 监听成功后（onReady）才启动首轮全量同步，避免同步期间丢失回调。
	syncDone := make(chan struct{})
	syncStarted := false
	startSync := func() {
		logger.Printf("HTTP 服务监听 %s（回调地址 http://<本机地址>%s/callback）", cfg.Addr(), cfg.Addr())
		syncStarted = true
		go func() {
			defer close(syncDone)
			sy.Run(ctx)
		}()
	}

	if err := srv.Run(ctx, startSync); err != nil {
		stop()
		if syncStarted {
			waitSync(syncDone, logger)
		}
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	}

	stop()
	waitSync(syncDone, logger)
	logger.Printf("服务已退出")
	return nil
}

func waitSync(done <-chan struct{}, logger *log.Logger) {
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		logger.Printf("同步任务未在 5 秒内退出，继续关闭")
	}
}

func dbDescription(cfg *config.Config) string {
	if cfg.DB != nil {
		return "mysql（DSN 已配置）"
	}
	return "sqlite " + cfg.ResolvedDBPath()
}
