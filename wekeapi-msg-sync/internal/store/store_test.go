package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// 旧库迁移依赖内部 columnExists，黑盒无法覆盖，测试留在本包。
func TestMigrationLegacyMessagesSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	// 构造旧库：messages 含 direction/dedupe_key 两列，且缺少 revoked 列。
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开旧库失败: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wx_id TEXT NOT NULL,
		direction INTEGER NOT NULL,
		new_msg_id TEXT NOT NULL DEFAULT '',
		dedupe_key TEXT NOT NULL,
		room_id TEXT NOT NULL DEFAULT '',
		create_time INTEGER NOT NULL DEFAULT 0,
		msg_type INTEGER NOT NULL DEFAULT 0,
		from_wxid TEXT NOT NULL DEFAULT '',
		to_wxid TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		raw_json TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(wx_id, dedupe_key)
	)`); err != nil {
		raw.Close()
		t.Fatalf("创建旧表失败: %v", err)
	}
	// 一条有 new_msg_id，一条只有 dedupe_key（旧库无消息 ID 时的内容哈希）。
	if _, err := raw.Exec(`INSERT INTO messages
		(wx_id, direction, new_msg_id, dedupe_key, create_time, msg_type, from_wxid, to_wxid, content, created_at, updated_at)
		VALUES ('app1', 2, '9100', '9100', 1700300000, 1, 'peer', 'self', '旧消息', '2024-01-01 00:00:00', '2024-01-01 00:00:00'),
		       ('app1', 1, '', 'h:legacy', 1700300001, 1, 'self', 'peer', '无ID旧消息', '2024-01-01 00:00:00', '2024-01-01 00:00:00')`); err != nil {
		raw.Close()
		t.Fatalf("写入旧数据失败: %v", err)
	}
	raw.Close()

	st, err := Open(ctx, Options{DBPath: path})
	if err != nil {
		t.Fatalf("迁移旧库失败: %v", err)
	}
	defer st.Close()

	// 旧列已删除（revoked 列由 ensureMessagesRevoked 补齐）。
	for _, col := range []string{"direction", "dedupe_key"} {
		exists, err := st.columnExists(ctx, "messages", col)
		if err != nil || exists {
			t.Fatalf("迁移后仍存在列 %s（err=%v）", col, err)
		}
	}

	// 历史数据保留，缺失的 new_msg_id 用 dedupe_key 回填。
	page, err := st.QueryMessages(ctx, MessageFilter{Owner: "app1", Page: 1, Limit: 10})
	if err != nil || page.Total != 2 {
		t.Fatalf("迁移后消息 total=%d err=%v, want 2", page.Total, err)
	}
	ids := map[string]bool{}
	for _, m := range page.Items {
		ids[m.NewMsgID] = true
	}
	if !ids["9100"] || !ids["h:legacy"] {
		t.Errorf("迁移后 new_msg_id = %v, want 9100 与 h:legacy", ids)
	}

	// 迁移后可正常写入与标记撤回。
	if _, err := st.InsertMessage(ctx, "app1", &MessageRecord{
		NewMsgID: "9101", CreateTime: 1700300002, MsgType: 1, FromWxID: "peer", ToWxID: "self", Content: "旧库消息",
	}); err != nil {
		t.Fatalf("旧库迁移后写入失败: %v", err)
	}
	if n, err := st.MarkMessageRevoked(ctx, "app1", "9101"); err != nil || n != 1 {
		t.Fatalf("旧库迁移后撤回 = %d, err=%v, want 1", n, err)
	}
}

func TestIsUnknownDatabase(t *testing.T) {
	if !isUnknownDatabase(&mysql.MySQLError{Number: 1049, Message: "Unknown database 'x'"}) {
		t.Error("1049 应判定为库不存在")
	}
	if !isUnknownDatabase(fmt.Errorf("包裹: %w", &mysql.MySQLError{Number: 1049})) {
		t.Error("包裹后的 1049 也应判定为库不存在")
	}
	if isUnknownDatabase(&mysql.MySQLError{Number: 1045}) {
		t.Error("1045 不应判定为库不存在")
	}
	if isUnknownDatabase(errors.New("boom")) {
		t.Error("普通错误不应判定为库不存在")
	}
}

func TestCreateMySQLDatabaseRequiresDBName(t *testing.T) {
	err := createMySQLDatabase(context.Background(), "user:pass@tcp(127.0.0.1:3306)/?charset=utf8mb4")
	if err == nil || !strings.Contains(err.Error(), "未指定数据库名") {
		t.Fatalf("DSN 无库名时应报错，实际 err=%v", err)
	}
}

func TestCreateMySQLDatabaseInvalidDSN(t *testing.T) {
	if err := createMySQLDatabase(context.Background(), "not-a-valid-dsn"); err == nil {
		t.Fatal("非法 DSN 应报错")
	}
}
