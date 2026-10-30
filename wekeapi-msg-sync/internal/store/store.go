// Package store 实现 wekeapi-msg-sync 的数据存储，同时支持 SQLite（默认）与 MySQL。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"wekeapi-msg-sync/internal/config"
)

// 联系人类型。
const (
	TypeFriend   = 1 // 好友
	TypeChatroom = 2 // 群聊
	TypeGh       = 3 // 公众号
	TypeWework   = 4 // 企微好友
)

// Dialect 数据库方言。
type Dialect int

const (
	SQLite Dialect = iota
	MySQL
)

func (d Dialect) String() string {
	if d == MySQL {
		return "mysql"
	}
	return "sqlite"
}

// Store 数据库存储。
type Store struct {
	db      *sql.DB
	dialect Dialect
}

// Options 打开数据库的选项。
type Options struct {
	DBPath string
	DB     *config.DBConfig
}

// Open 打开数据库并执行建表迁移。
func Open(ctx context.Context, opts Options) (*Store, error) {
	var (
		db      *sql.DB
		dialect Dialect
		err     error
	)
	if opts.DB != nil {
		dialect = MySQL
		db, err = sql.Open("mysql", opts.DB.DSN)
		if err != nil {
			return nil, fmt.Errorf("打开 MySQL 失败: %w", err)
		}
		db.SetMaxOpenConns(opts.DB.MaxOpenConns)
		db.SetMaxIdleConns(opts.DB.MaxIdleConns)
		db.SetConnMaxLifetime(time.Duration(opts.DB.ConnMaxLifetimeSeconds) * time.Second)
	} else {
		dialect = SQLite
		path := opts.DBPath
		if path == "" {
			path = config.DefaultDBPath
		}
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("创建数据库目录 %s 失败: %w", dir, err)
			}
		}
		dsn := path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
		db, err = sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("打开 SQLite %s 失败: %w", path, err)
		}
	}
	if err != nil {
		return nil, err
	}

	// MySQL 首次连接时若目标库不存在（错误码 1049），自动建库后重试，
	// 用户无需手工执行 CREATE DATABASE。
	if err := pingDB(ctx, db); err != nil {
		if dialect != MySQL || opts.DB == nil || !isUnknownDatabase(err) {
			db.Close()
			return nil, fmt.Errorf("连接数据库失败（%s）: %w", dialect, err)
		}
		if cerr := createMySQLDatabase(ctx, opts.DB.DSN); cerr != nil {
			db.Close()
			return nil, cerr
		}
		if err := pingDB(ctx, db); err != nil {
			db.Close()
			return nil, fmt.Errorf("连接数据库失败（%s）: %w", dialect, err)
		}
	}

	s := &Store{db: db, dialect: dialect}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// pingDB 在固定超时内探测数据库连通性。
func pingDB(ctx context.Context, db *sql.DB) error {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(pingCtx)
}

// isUnknownDatabase 判断错误是否为 MySQL「库不存在」(1049)。
func isUnknownDatabase(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1049
	}
	return false
}

// createMySQLDatabase 用 DSN 中的账号连接服务端（不指定库），按 DSN 里的库名建库。
// 建库失败通常意味着账号缺少 CREATE 权限，错误中给出可执行的提示。
func createMySQLDatabase(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("解析 MySQL 连接串失败: %w", err)
	}
	name := strings.TrimSpace(cfg.DBName)
	if name == "" {
		return fmt.Errorf("连接串未指定数据库名，无法自动建库：请在 DSN 中补上 /dbname")
	}

	serverCfg := *cfg
	serverCfg.DBName = ""
	server, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("连接 MySQL 服务失败: %w", err)
	}
	defer server.Close()

	ddl := "CREATE DATABASE IF NOT EXISTS `" + strings.ReplaceAll(name, "`", "``") +
		"` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"
	if _, err := server.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("数据库 %s 不存在且自动创建失败（请确认账号具备 CREATE 权限，或手工建库）: %w", name, err)
	}
	return nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Dialect 返回当前数据库方言。
func (s *Store) Dialect() Dialect { return s.dialect }

func (s *Store) migrate(ctx context.Context) error {
	stmts := sqliteDDL
	if s.dialect == MySQL {
		stmts = mysqlDDL
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("建表失败（%s）: %w", s.dialect, err)
		}
	}
	if err := s.ensureMessagesRevoked(ctx); err != nil {
		return err
	}
	return s.migrateMessagesUnique(ctx)
}

// ensureMessagesRevoked 为旧库补充 messages.revoked 列（撤回状态）。
func (s *Store) ensureMessagesRevoked(ctx context.Context) error {
	exists, err := s.columnExists(ctx, "messages", "revoked")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	stmt := `ALTER TABLE messages ADD COLUMN revoked INTEGER NOT NULL DEFAULT 0`
	if s.dialect == MySQL {
		stmt = `ALTER TABLE messages ADD COLUMN revoked TINYINT NOT NULL DEFAULT 0`
	}
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("迁移 messages.revoked 失败: %w", err)
	}
	return nil
}

// migrateMessagesUnique 升级旧库 messages 表：去掉 dedupe_key / direction 两列，
// 改用 (wx_id, new_msg_id) 唯一约束（收发方向改为按 from_wxid 是否等于本账号判断）。
func (s *Store) migrateMessagesUnique(ctx context.Context) error {
	hasDedupe, err := s.columnExists(ctx, "messages", "dedupe_key")
	if err != nil {
		return err
	}
	hasDirection, err := s.columnExists(ctx, "messages", "direction")
	if err != nil {
		return err
	}
	if !hasDedupe && !hasDirection {
		return nil
	}
	if s.dialect == MySQL {
		return s.migrateMessagesMySQL(ctx, hasDedupe, hasDirection)
	}
	return s.migrateMessagesSQLite(ctx, hasDedupe)
}

// migrateMessagesMySQL 就地修改 MySQL 旧表：回填 new_msg_id、删除旧列与旧唯一键，补新唯一键。
func (s *Store) migrateMessagesMySQL(ctx context.Context, hasDedupe, hasDirection bool) error {
	if hasDedupe {
		// 无 new_msg_id 的历史数据用 dedupe_key 回填（旧库 dedupe_key 缺失时就是内容哈希）。
		if _, err := s.db.ExecContext(ctx,
			`UPDATE messages SET new_msg_id = dedupe_key WHERE new_msg_id = ''`); err != nil {
			return fmt.Errorf("迁移 messages.new_msg_id 失败: %w", err)
		}
		exists, err := s.indexExists(ctx, "messages", "uk_messages_dedupe")
		if err != nil {
			return err
		}
		if exists {
			if _, err := s.db.ExecContext(ctx, `ALTER TABLE messages DROP INDEX uk_messages_dedupe`); err != nil {
				return fmt.Errorf("删除唯一键 uk_messages_dedupe 失败: %w", err)
			}
		}
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE messages DROP COLUMN dedupe_key`); err != nil {
			return fmt.Errorf("删除 messages.dedupe_key 失败: %w", err)
		}
	}
	if hasDirection {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE messages DROP COLUMN direction`); err != nil {
			return fmt.Errorf("删除 messages.direction 失败: %w", err)
		}
	}
	exists, err := s.indexExists(ctx, "messages", "uk_messages_msgid")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE messages ADD UNIQUE KEY uk_messages_msgid (wx_id, new_msg_id)`); err != nil {
			return fmt.Errorf("创建唯一键 uk_messages_msgid 失败: %w", err)
		}
	}
	return nil
}

// migrateMessagesSQLite 重建 SQLite 旧表：SQLite 无法在保留唯一约束的情况下删列，只能复制数据重建。
func (s *Store) migrateMessagesSQLite(ctx context.Context, hasDedupe bool) error {
	// 旧库 new_msg_id 可能为空，此时用 dedupe_key 回填，避免新唯一键把多条空 ID 消息合并。
	newMsgExpr := "new_msg_id"
	if hasDedupe {
		newMsgExpr = "CASE WHEN TRIM(new_msg_id) <> '' THEN new_msg_id ELSE dedupe_key END"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启迁移事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交成功后 rollback 无效

	stmts := []string{
		`ALTER TABLE messages RENAME TO messages_legacy`,
		sqliteMessagesTableDDL,
		`INSERT INTO messages (id, wx_id, new_msg_id, room_id, create_time, msg_type,
			from_wxid, to_wxid, content, raw_json, revoked, created_at, updated_at)
		SELECT id, wx_id, ` + newMsgExpr + `, room_id, create_time, msg_type,
			from_wxid, to_wxid, content, raw_json, revoked, created_at, updated_at FROM messages_legacy`,
		`DROP TABLE messages_legacy`,
		`CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(wx_id, room_id, create_time)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_time ON messages(wx_id, create_time)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("迁移 messages 表失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 messages 迁移失败: %w", err)
	}
	return nil
}

// indexExists 判断索引是否存在（仅 MySQL 需要，SQLite 迁移走整表重建）。
func (s *Store) indexExists(ctx context.Context, table, index string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, index).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("检查 MySQL 索引 %s.%s 失败: %w", table, index, err)
	}
	return n > 0, nil
}

func (s *Store) columnExists(ctx context.Context, table, column string) (bool, error) {
	if s.dialect == MySQL {
		var n int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("检查 MySQL 列 %s.%s 失败: %w", table, column, err)
		}
		return n > 0, nil
	}

	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("检查 SQLite 列 %s.%s 失败: %w", table, column, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, colType    string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("读取 SQLite 表结构失败: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// insertIgnore 返回当前方言的「冲突时忽略」插入前缀。
func (s *Store) insertIgnore(table string) string {
	if s.dialect == MySQL {
		return "INSERT IGNORE INTO " + table
	}
	return "INSERT OR IGNORE INTO " + table
}

func nowString() string {
	return time.Now().UTC().Format(time.DateTime)
}

// normalizeOwner 清理归属键。
func normalizeOwner(owner string) string { return strings.TrimSpace(owner) }
