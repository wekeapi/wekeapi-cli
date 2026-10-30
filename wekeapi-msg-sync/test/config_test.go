package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wekeapi-msg-sync/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.DefaultFileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, `{"token":"tk","appId":"app","wxid":"wxid_1"}`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.BaseURL != config.DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, config.DefaultBaseURL)
	}
	if cfg.Port != config.DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, config.DefaultPort)
	}
	if cfg.DBPath != config.DefaultDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, config.DefaultDBPath)
	}
	if cfg.DB != nil {
		t.Errorf("DB = %+v, want nil", cfg.DB)
	}
	if cfg.DBKind() != "sqlite" {
		t.Errorf("DBKind = %q, want sqlite", cfg.DBKind())
	}
}

func TestLoadMutualExclusion(t *testing.T) {
	path := writeConfig(t, `{
		"token":"tk","appId":"app","wxid":"wxid_1",
		"db_path":"./x.db",
		"db":{"driver":"mysql","dsn":"user:pass@tcp(127.0.0.1:3306)/db"}
	}`)
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("期望 db_path/db 互斥错误，实际: %v", err)
	}
}

func TestLoadMySQLDefaults(t *testing.T) {
	path := writeConfig(t, `{
		"token":"tk","appId":"app","wxid":"wxid_1",
		"db":{"driver":"MySQL","dsn":"user:pass@tcp(127.0.0.1:3306)/db"}
	}`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.DB == nil {
		t.Fatal("DB == nil")
	}
	if cfg.DB.Driver != "mysql" {
		t.Errorf("Driver = %q, want mysql", cfg.DB.Driver)
	}
	if cfg.DB.MaxOpenConns != config.DefaultMaxOpenConns || cfg.DB.MaxIdleConns != config.DefaultMaxIdleConns ||
		cfg.DB.ConnMaxLifetimeSeconds != config.DefaultConnMaxLifetimeSeconds {
		t.Errorf("连接池默认值错误: %+v", cfg.DB)
	}
	if cfg.DBPath != "" {
		t.Errorf("DBPath = %q, want empty", cfg.DBPath)
	}
	if cfg.DBKind() != "mysql" {
		t.Errorf("DBKind = %q, want mysql", cfg.DBKind())
	}
}

func TestLoadRejectsSQLiteDriver(t *testing.T) {
	path := writeConfig(t, `{
		"token":"tk","appId":"app","wxid":"wxid_1",
		"db":{"driver":"sqlite","dsn":"./x.db"}
	}`)
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "仅支持 mysql") {
		t.Fatalf("期望 driver 校验错误，实际: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望文件不存在错误，实际: %v", err)
	}
}

func TestDefaultPathUnderHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取用户主目录")
	}
	wantHome := filepath.Join(home, ".wekeapi", "msg-sync")
	if got := config.DefaultHome(); got != wantHome {
		t.Fatalf("DefaultHome = %q, want %q", got, wantHome)
	}
	wantPath := filepath.Join(wantHome, config.DefaultFileName)
	if got := config.DefaultPath(); got != wantPath {
		t.Fatalf("DefaultPath = %q, want %q", got, wantPath)
	}
}

func TestResolvedDBPathRelativeToConfigDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultFileName)

	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"相对路径", &config.Config{DBPath: "data.db", Path: path}, filepath.Join(dir, "data.db")},
		{"默认路径", &config.Config{DBPath: config.DefaultDBPath, Path: path}, filepath.Join(dir, "wekeapi_msg_sync.db")},
		{"绝对路径", &config.Config{DBPath: filepath.Join(dir, "abs.db"), Path: path}, filepath.Join(dir, "abs.db")},
		{"MySQL", &config.Config{DB: &config.DBConfig{Driver: "mysql", DSN: "u:p@tcp(h:3306)/d"}, Path: path}, ""},
	}
	for _, c := range cases {
		if got := c.cfg.ResolvedDBPath(); got != c.want {
			t.Fatalf("%s: ResolvedDBPath = %q, want %q", c.name, got, c.want)
		}
	}
}
