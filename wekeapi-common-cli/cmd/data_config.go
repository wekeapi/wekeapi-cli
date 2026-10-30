// data config：设置数据同步服务的数据库（SQLite/MySQL）与服务端口。
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/msgservice"
	"wekeapi-cli/internal/runner"
)

// newDataConfigCmd 构建 data config 命令。
func newDataConfigCmd(opts *runner.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "设置数据同步服务的数据库（SQLite/MySQL）与端口",
		Long: `设置 wekeapi-msg-sync 的数据库与服务端口（写入服务目录的 wekeapi-msg-sync.json）。

数据库二选一：
  SQLite（默认）：--db sqlite --db-path <文件名>，默认 wekeapi_msg_sync.db（相对服务目录）；
  MySQL：--db mysql --db-dsn <连接串>，支持以下两种写法：
         user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4
         mysql://user:pass@127.0.0.1:3306/dbname?charset=utf8mb4
         数据库不存在时服务启动会自动创建（账号需具备 CREATE 权限），表同样自动创建。

端口：--port <1-65535>，默认 3006。

不带 --db/--db-path/--db-dsn/--port 时只查看当前配置，不写文件。
配置修改后需重启服务生效：wekeapi data sync（重启后自动全量同步）。
切换数据库类型不会迁移旧库数据。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := resolveMsgLayout(cmd)
			if err != nil {
				return failData(opts, err)
			}
			db, _ := cmd.Flags().GetString("db")
			dbPath, _ := cmd.Flags().GetString("db-path")
			dsn, _ := cmd.Flags().GetString("db-dsn")
			port, _ := cmd.Flags().GetInt("port")

			// 只给 --db-path 或 --db-dsn 时按参数推断数据库类型。
			if strings.TrimSpace(db) == "" {
				switch {
				case strings.TrimSpace(dsn) != "":
					db = "mysql"
				case strings.TrimSpace(dbPath) != "":
					db = "sqlite"
				}
			}

			res, err := msgservice.Configure(cmd.Context(), layout, msgservice.ConfigureOptions{
				DB:     db,
				DBPath: dbPath,
				DSN:    dsn,
				Port:   port,
			})
			if err != nil {
				return failData(opts, err)
			}
			if res.Changed {
				fmt.Fprintf(opts.Stderr, "已更新数据同步服务配置：%s\n", res.ConfigPath)
			}
			if res.Hint != "" {
				fmt.Fprintf(opts.Stderr, "[msg-sync] %s\n", res.Hint)
			}
			return writeLocalResult(opts, res)
		},
	}
	cmd.Flags().String("db", "", "数据库类型：sqlite / mysql")
	cmd.Flags().String("db-path", "", "SQLite 数据库文件名，默认 wekeapi_msg_sync.db（相对服务目录）")
	cmd.Flags().String("db-dsn", "", "MySQL 连接串（DSN 或 mysql:// URI）")
	cmd.Flags().Int("port", 0, "服务端口，默认 3006")
	return cmd
}
