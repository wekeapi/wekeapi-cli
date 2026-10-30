package msgservice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigPreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	seed := `{
  "token": "old-token",
  "appId": "old-app",
  "baseURL": "https://old.example/api",
  "wxid": "old-wxid",
  "port": 3555,
  "db": {"driver": "mysql", "dsn": "user:pass@tcp(127.0.0.1:3306)/db"},
  "unknown_field": "keep-me"
}
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	changed, cfg, err := WriteConfig(path, Settings{
		Token:   "new-token",
		AppID:   "new-app",
		BaseURL: "https://api-test.wekeapi.com/api/",
		WxID:    "new-wxid",
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	if !changed {
		t.Fatalf("凭证变化时应返回 changed=true")
	}
	if cfg.Port != 3555 || cfg.DBKind != "mysql" || cfg.WxID != "new-wxid" {
		t.Fatalf("核心字段不符: %+v", cfg)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["token"]) != `"new-token"` || string(raw["appId"]) != `"new-app"` {
		t.Fatalf("token/appId 未更新: %s", string(b))
	}
	if string(raw["baseURL"]) != `"https://api-test.wekeapi.com/api"` {
		t.Fatalf("baseURL 应去掉末尾斜杠: %s", string(raw["baseURL"]))
	}
	if string(raw["db"]) == "" || string(raw["unknown_field"]) != `"keep-me"` {
		t.Fatalf("db/未知字段应保留: %s", string(b))
	}

	// 相同凭证再次写入：内容不变，changed=false（避免 login status 反复重启服务）。
	changed, _, err = WriteConfig(path, Settings{
		Token:   "new-token",
		AppID:   "new-app",
		BaseURL: "https://api-test.wekeapi.com/api",
		WxID:    "new-wxid",
	})
	if err != nil {
		t.Fatalf("WriteConfig second: %v", err)
	}
	if changed {
		t.Fatalf("凭证未变化时应返回 changed=false")
	}
}

func TestWriteConfigCreatesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	changed, cfg, err := WriteConfig(path, Settings{
		Token:   "t",
		AppID:   "a",
		BaseURL: "https://api.wekeapi.com/api",
		WxID:    "w",
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	if !changed {
		t.Fatalf("新文件应返回 changed=true")
	}
	if cfg.Port != DefaultPort || cfg.DBKind != "sqlite" {
		t.Fatalf("默认值不符: %+v", cfg)
	}

	b, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["db_path"]) != `"./wekeapi_msg_sync.db"` {
		t.Fatalf("db_path 默认值不符: %s", string(raw["db_path"]))
	}
	if _, ok := raw["sync_interval"]; ok {
		t.Fatalf("已移除的 sync_interval 不应再写入: %s", string(b))
	}
}

func TestLoadConfigRejectsBrokenJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteConfig(path, Settings{Token: "t", AppID: "a", BaseURL: "https://x/api", WxID: "w"}); err == nil {
		t.Fatalf("损坏的配置应报错而不是覆盖")
	}
}

func newTestLayout(t *testing.T) (*Layout, string) {
	t.Helper()
	dir := t.TempDir()
	return &Layout{
		Home:       dir,
		ConfigPath: filepath.Join(dir, ConfigFileName),
		PIDPath:    filepath.Join(dir, PIDFileName),
		LogPath:    filepath.Join(dir, LogFileName),
	}, filepath.Join(dir, ConfigFileName)
}

func readRaw(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return raw
}

func TestConfigureSQLiteClearsMySQLAndKeepsCredentials(t *testing.T) {
	layout, path := newTestLayout(t)
	seed := `{
  "token": "t",
  "appId": "a",
  "baseURL": "https://api.wekeapi.com/api",
  "wxid": "w",
  "port": 3555,
  "db": {"driver": "mysql", "dsn": "user:pass@tcp(127.0.0.1:3306)/db", "max_open_conns": 20}
}
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Configure(context.Background(), layout, ConfigureOptions{DB: "sqlite", DBPath: "my_data.db", Port: 3911})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !res.Changed || res.DB != "sqlite" || res.DBPath != "my_data.db" || res.Port != 3911 {
		t.Fatalf("结果不符: %+v", res)
	}
	if res.NeedRestart {
		t.Fatalf("服务未运行时不需重启: %+v", res)
	}

	raw := readRaw(t, path)
	if _, ok := raw["db"]; ok {
		t.Fatalf("切到 SQLite 应删除 db 配置: %s", string(raw["db"]))
	}
	if string(raw["db_path"]) != `"my_data.db"` {
		t.Fatalf("db_path 不符: %s", string(raw["db_path"]))
	}
	if string(raw["port"]) != "3911" || string(raw["token"]) != `"t"` {
		t.Fatalf("port/token 不符: %s %s", string(raw["port"]), string(raw["token"]))
	}
}

func TestConfigureMySQLFromURI(t *testing.T) {
	layout, path := newTestLayout(t)
	seed := `{"token":"t","appId":"a","baseURL":"https://x/api","wxid":"w","db_path":"./old.db"}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Configure(context.Background(), layout, ConfigureOptions{
		DB:  "mysql",
		DSN: "mysql://user:p%40ss@127.0.0.1:3306/wekeapi?charset=utf8mb4",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if res.DB != "mysql" {
		t.Fatalf("数据库类型不符: %+v", res)
	}
	if strings.Contains(res.DSN, "p@ss") || !strings.Contains(res.DSN, "***") {
		t.Fatalf("输出中的 DSN 应脱敏: %s", res.DSN)
	}

	raw := readRaw(t, path)
	if _, ok := raw["db_path"]; ok {
		t.Fatalf("切到 MySQL 应删除 db_path: %s", string(raw["db_path"]))
	}
	var db map[string]any
	if err := json.Unmarshal(raw["db"], &db); err != nil {
		t.Fatalf("db 字段不是对象: %v", err)
	}
	if db["driver"] != "mysql" {
		t.Fatalf("driver 不符: %v", db["driver"])
	}
	dsn, _ := db["dsn"].(string)
	if !strings.HasPrefix(dsn, "user:p@ss@tcp(127.0.0.1:3306)/wekeapi?") || !strings.Contains(dsn, "parseTime=true") {
		t.Fatalf("DSN 转换不符: %s", dsn)
	}
}

func TestConfigureViewOnlyDoesNotChangeFile(t *testing.T) {
	layout, path := newTestLayout(t)
	seed := `{"token":"t","appId":"a","baseURL":"https://x/api","wxid":"w","port":3777,"db_path":"a.db"}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Configure(context.Background(), layout, ConfigureOptions{})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if res.Changed || res.Port != 3777 || res.DB != "sqlite" || res.DBPath != "a.db" {
		t.Fatalf("仅查询结果不符: %+v", res)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != seed {
		t.Fatalf("仅查询不应改动文件: %s (%v)", string(b), err)
	}
}

func TestConfigureRejectsInvalidInput(t *testing.T) {
	layout, path := newTestLayout(t)
	seed := `{"token":"t","appId":"a","baseURL":"https://x/api","wxid":"w","db_path":"a.db"}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Configure(context.Background(), layout, ConfigureOptions{DB: "postgres"}); err == nil {
		t.Fatalf("不支持的数据库类型应报错")
	}
	if _, err := Configure(context.Background(), layout, ConfigureOptions{Port: 70000}); err == nil {
		t.Fatalf("越界端口应报错")
	}
	if _, err := Configure(context.Background(), layout, ConfigureOptions{DB: "mysql"}); err == nil {
		t.Fatalf("MySQL 缺连接串应报错")
	}
}

func TestEnsureAsksDatabaseOnFirstRun(t *testing.T) {
	layout, path := newTestLayout(t)
	if _, err := layout.BinaryPath(); err == nil {
		t.Skip("环境中存在 wekeapi-msg-sync 可执行文件，跳过以避免启动真实服务")
	}

	asked := 0
	_, err := Ensure(context.Background(), layout, EnsureOptions{
		Settings: Settings{Token: "t", AppID: "a", BaseURL: "https://x/api", WxID: "w"},
		AskDB: func() (*ConfigureOptions, error) {
			asked++
			// 用非默认端口，避免探测到本机真实运行的服务
			return &ConfigureOptions{DB: "mysql", DSN: "mysql://user:pass@127.0.0.1:3306/wekeapi", Port: 35999}, nil
		},
	})
	if asked != 1 {
		t.Fatalf("首次创建配置应询问一次数据库类型，实际 %d 次", asked)
	}
	if err == nil {
		t.Fatalf("缺少服务可执行文件时应返回错误")
	}

	raw := readRaw(t, path)
	if _, ok := raw["db_path"]; ok {
		t.Fatalf("选择 MySQL 时不应写入 db_path: %s", string(raw["db_path"]))
	}
	var db map[string]any
	if err := json.Unmarshal(raw["db"], &db); err != nil {
		t.Fatalf("db 字段不是对象: %v", err)
	}
	if db["driver"] != "mysql" {
		t.Fatalf("driver 应为 mysql: %v", db["driver"])
	}
	if string(raw["token"]) != `"t"` || string(raw["wxid"]) != `"w"` || string(raw["port"]) != "35999" {
		t.Fatalf("凭证/端口应与选择合并保留: %s", string(raw["port"]))
	}
}

func TestEnsureSkipsAskWhenConfigExists(t *testing.T) {
	layout, path := newTestLayout(t)
	if _, err := layout.BinaryPath(); err == nil {
		t.Skip("环境中存在 wekeapi-msg-sync 可执行文件，跳过以避免启动真实服务")
	}
	seed := `{"token":"t","appId":"a","baseURL":"https://x/api","wxid":"w","port":35998,"db_path":"a.db"}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	asked := 0
	_, err := Ensure(context.Background(), layout, EnsureOptions{
		Settings: Settings{Token: "t", AppID: "a", BaseURL: "https://x/api", WxID: "w"},
		AskDB: func() (*ConfigureOptions, error) {
			asked++
			return &ConfigureOptions{DB: "mysql", DSN: "mysql://user:pass@127.0.0.1:3306/wekeapi"}, nil
		},
	})
	if asked != 0 {
		t.Fatalf("配置已存在时不应询问数据库类型，实际 %d 次", asked)
	}
	if err == nil {
		t.Fatalf("缺少服务可执行文件时应返回错误")
	}
	raw := readRaw(t, path)
	if string(raw["db_path"]) != `"a.db"` {
		t.Fatalf("已存在的数据库配置不应被覆盖: %s", string(raw["db_path"]))
	}
}

func TestDSNFromURI(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"mysql://u:p@h:3306/db", "u:p@tcp(h:3306)/db?parseTime=true", false},
		{"mysql://u:p@h:3306/db?charset=utf8mb4", "u:p@tcp(h:3306)/db?charset=utf8mb4&parseTime=true", false},
		{"jdbc:mysql://root:root@127.0.0.1:3306/wekeapi_msg_sync?useSSL=false&serverTimezone=Asia/Shanghai&characterEncoding=UTF-8",
			"root:root@tcp(127.0.0.1:3306)/wekeapi_msg_sync?charset=utf8mb4&loc=Asia%2FShanghai&parseTime=true", false},
		{"jdbc:mysql://u:p@h:3306/db?useSSL=true", "u:p@tcp(h:3306)/db?parseTime=true&tls=true", false},
		{"jdbc:mysql://u:p@h:3306", "", true},
		{"u:p@tcp(h:3306)/db", "u:p@tcp(h:3306)/db?parseTime=true", false},
		{"u:p@tcp(h:3306)/db?parseTime=true", "u:p@tcp(h:3306)/db?parseTime=true", false},
		{"mysql://u:p@h:3306", "", true},
		{"not-a-dsn", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := DSNFromURI(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("%q 应报错，实际为 %q", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("%q -> %q (%v)，期望 %q", c.in, got, err, c.want)
		}
	}
}

func TestMaskDSN(t *testing.T) {
	if got := MaskDSN("u:pass@tcp(h:3306)/db"); got != "u:***@tcp(h:3306)/db" {
		t.Fatalf("MaskDSN 不符: %s", got)
	}
	if got := MaskDSN(""); got != "" {
		t.Fatalf("空 DSN 应原样返回: %q", got)
	}
}
