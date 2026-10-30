// Package msgservice 对接 wekeapi-msg-sync 数据同步服务：
//  1. 把主 CLI 的 token/appId/baseURL/wxid 同步写入 wekeapi-msg-sync.json；
//  2. 启动/停止/重启服务进程（Ensure 仅在未运行或凭证变化时启动/重启）；
//  3. 封装服务的 REST 查询 API，供 wekeapi data 命令使用。
package msgservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// 默认值（与 wekeapi-msg-sync 保持一致）。
const (
	DefaultPort   = 3006
	DefaultDBPath = "./wekeapi_msg_sync.db"
)

// Settings 从主 CLI 配置同步到服务配置的字段。
type Settings struct {
	Token   string
	AppID   string
	BaseURL string
	WxID    string
}

// ServiceConfig 服务配置中 CLI 关心的字段（用于确定端口/数据库类型）。
type ServiceConfig struct {
	Token   string
	AppID   string
	BaseURL string
	WxID    string
	Port    int
	DBKind  string // sqlite / mysql
	DBPath  string // SQLite 数据库文件（DBKind=sqlite）
	DSN     string // MySQL 连接串（DBKind=mysql）
}

// LoadConfig 读取服务配置文件的核心字段。
func LoadConfig(path string) (*ServiceConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("数据同步服务配置 %s 不存在：请先登录一次（登录成功或 login status 检测到在线时，会自动生成配置并启动服务）", path)
		}
		return nil, fmt.Errorf("读取数据同步服务配置 %s 失败: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("数据同步服务配置 %s 不是合法 JSON: %w", path, err)
	}
	cfg := &ServiceConfig{Port: DefaultPort, DBKind: "sqlite"}
	decodeString(raw, "token", &cfg.Token)
	decodeString(raw, "appId", &cfg.AppID)
	decodeString(raw, "baseURL", &cfg.BaseURL)
	decodeString(raw, "wxid", &cfg.WxID)
	if v, ok := raw["port"]; ok {
		var port int
		if err := json.Unmarshal(v, &port); err == nil && port > 0 {
			cfg.Port = port
		}
	}
	decodeString(raw, "db_path", &cfg.DBPath)
	if v, ok := raw["db"]; ok && len(bytes.TrimSpace(v)) > 0 && string(bytes.TrimSpace(v)) != "null" {
		cfg.DBKind = "mysql"
		var db map[string]json.RawMessage
		if err := json.Unmarshal(v, &db); err == nil {
			decodeString(db, "dsn", &cfg.DSN)
		}
	}
	return cfg, nil
}

// WriteConfig 把 Settings 合并写入服务配置：
// 仅更新 token/appId/baseURL/wxid，保留 port/db/db_path 与未知字段；
// 新文件补全默认值。返回配置是否发生变化。
func WriteConfig(path string, s Settings) (changed bool, cfg *ServiceConfig, err error) {
	raw, oldRaw, err := loadRawConfig(path)
	if err != nil {
		return false, nil, err
	}

	if err := setString(raw, "token", s.Token); err != nil {
		return false, nil, err
	}
	if err := setString(raw, "appId", s.AppID); err != nil {
		return false, nil, err
	}
	if err := setString(raw, "baseURL", strings.TrimRight(s.BaseURL, "/")); err != nil {
		return false, nil, err
	}
	if err := setString(raw, "wxid", s.WxID); err != nil {
		return false, nil, err
	}
	if _, ok := raw["port"]; !ok {
		if err := setValue(raw, "port", DefaultPort); err != nil {
			return false, nil, err
		}
	}
	if _, ok := raw["db"]; !ok {
		if _, hasPath := raw["db_path"]; !hasPath {
			if err := setValue(raw, "db_path", DefaultDBPath); err != nil {
				return false, nil, err
			}
		}
	}

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return false, nil, fmt.Errorf("序列化数据同步服务配置失败: %w", err)
	}
	out = append(out, '\n')
	changed = !bytes.Equal(bytes.TrimSpace(oldRaw), bytes.TrimSpace(out))
	if changed {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return false, nil, fmt.Errorf("创建数据同步服务配置目录失败: %w", err)
		}
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return false, nil, fmt.Errorf("写入数据同步服务配置 %s 失败: %w%s", path, err, pathErrHint(err))
		}
	}
	cfg, err = LoadConfig(path)
	if err != nil {
		return false, nil, err
	}
	return changed, cfg, nil
}

// pathErrHint 为权限类错误追加可操作的排查提示：CLI 从被安全软件
// 限制写入的目录运行（如非系统盘的 workspace）时，写用户目录会得到
// Access is denied，此时应改用 --msg-sync-dir 或迁移 CLI 到可写目录。
func pathErrHint(err error) string {
	if os.IsPermission(err) {
		return "（权限不足：若 CLI 位于受限目录或无法写入用户目录，可改用 --msg-sync-dir 指定可写目录，或把 CLI 复制到用户目录后运行）"
	}
	return ""
}

func decodeString(raw map[string]json.RawMessage, key string, dst *string) {
	v, ok := raw[key]
	if !ok {
		return
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		*dst = s
	}
}

func setString(raw map[string]json.RawMessage, key, value string) error {
	return setValue(raw, key, value)
}

func setValue(raw map[string]json.RawMessage, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化配置字段 %s 失败: %w", key, err)
	}
	raw[key] = b
	return nil
}

// loadRawConfig 把配置文件读成原始 JSON 对象；文件不存在时返回空对象与 nil（旧内容为 nil）。
func loadRawConfig(path string) (map[string]json.RawMessage, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil, nil
		}
		return nil, nil, fmt.Errorf("读取数据同步服务配置 %s 失败: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, nil, fmt.Errorf("数据同步服务配置 %s 不是合法 JSON（请修复或删除后重新登录）: %w", path, err)
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	return raw, b, nil
}

// ConfigureOptions 数据库与端口设置；字段为零值时表示不修改。
type ConfigureOptions struct {
	DB     string // sqlite / mysql；空表示不修改数据库配置（只查询时也为空）
	DBPath string // SQLite 文件名/路径（DB=sqlite 时使用，空则保留原值）
	DSN    string // MySQL 连接串（DB=mysql 时使用），支持 DSN 或 mysql:// URI
	Port   int    // 服务端口；0 表示不修改
}

// ConfigureResult 配置结果（输出为 JSON）。
type ConfigureResult struct {
	ConfigPath  string `json:"configPath"`
	Port        int    `json:"port"`
	DB          string `json:"db"`
	DBPath      string `json:"db_path,omitempty"`
	DSN         string `json:"dsn,omitempty"` // 已脱敏
	Changed     bool   `json:"changed"`
	NeedRestart bool   `json:"needRestart"`
	Hint        string `json:"hint,omitempty"`
}

// Configure 只更新数据库与端口（保留 token/appId/wxid 等其余字段），
// 返回写入后的当前配置；Changed=false 表示内容未变化（仅查询时也是 false）。
func Configure(ctx context.Context, layout *Layout, opts ConfigureOptions) (*ConfigureResult, error) {
	kind := strings.ToLower(strings.TrimSpace(opts.DB))
	if kind != "" && kind != "sqlite" && kind != "mysql" {
		return nil, fmt.Errorf("数据库类型仅支持 sqlite 或 mysql，当前为 %q", opts.DB)
	}
	if opts.Port != 0 && (opts.Port < 1 || opts.Port > 65535) {
		return nil, fmt.Errorf("端口必须在 1-65535 之间，当前为 %d", opts.Port)
	}

	raw, oldRaw, err := loadRawConfig(layout.ConfigPath)
	if err != nil {
		return nil, err
	}

	// 未指定任何修改项时只返回当前配置，不改动文件。
	viewOnly := kind == "" && opts.Port == 0

	switch kind {
	case "sqlite":
		dbPath := strings.TrimSpace(opts.DBPath)
		if dbPath == "" {
			var old string
			decodeString(raw, "db_path", &old)
			dbPath = strings.TrimSpace(old)
		}
		if dbPath == "" {
			dbPath = DefaultDBPath
		}
		delete(raw, "db")
		if err := setValue(raw, "db_path", dbPath); err != nil {
			return nil, err
		}
	case "mysql":
		db, err := mysqlConfig(raw, opts.DSN)
		if err != nil {
			return nil, err
		}
		delete(raw, "db_path")
		if err := setValue(raw, "db", db); err != nil {
			return nil, err
		}
	}
	if opts.Port > 0 {
		if err := setValue(raw, "port", opts.Port); err != nil {
			return nil, err
		}
	}

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化数据同步服务配置失败: %w", err)
	}
	out = append(out, '\n')
	changed := false
	if !viewOnly {
		changed = !bytes.Equal(bytes.TrimSpace(oldRaw), bytes.TrimSpace(out))
		if changed {
			if err := os.MkdirAll(filepath.Dir(layout.ConfigPath), 0o700); err != nil {
				return nil, fmt.Errorf("创建数据同步服务配置目录失败: %w", err)
			}
			if err := os.WriteFile(layout.ConfigPath, out, 0o600); err != nil {
				return nil, fmt.Errorf("写入数据同步服务配置 %s 失败: %w%s", layout.ConfigPath, err, pathErrHint(err))
			}
		}
	}

	cfg, err := LoadConfig(layout.ConfigPath)
	if err != nil {
		return nil, err
	}
	res := &ConfigureResult{
		ConfigPath: layout.ConfigPath,
		Port:       cfg.Port,
		DB:         cfg.DBKind,
		DBPath:     cfg.DBPath,
		DSN:        MaskDSN(cfg.DSN),
		Changed:    changed,
	}
	if cfg.DBKind == "sqlite" && res.DBPath == "" {
		res.DBPath = DefaultDBPath
	}

	running, _ := probe(ctx, layout, servicePort(layout))
	switch {
	case changed && running:
		res.NeedRestart = true
		res.Hint = "配置已保存，但正在运行的服务仍在使用旧配置：执行 wekeapi data sync 重启后生效"
	case changed:
		res.Hint = "配置已保存，服务启动后按该配置运行（wekeapi data start 或登录成功后自动启动）"
	case running:
		res.Hint = "配置无变化，服务运行中"
	}
	return res, nil
}

// mysqlConfig 生成 MySQL 的 db 配置对象，保留已有的连接池参数。
func mysqlConfig(raw map[string]json.RawMessage, dsn string) (map[string]json.RawMessage, error) {
	db := map[string]json.RawMessage{}
	if v, ok := raw["db"]; ok && len(bytes.TrimSpace(v)) > 0 && string(bytes.TrimSpace(v)) != "null" {
		if err := json.Unmarshal(v, &db); err != nil {
			return nil, fmt.Errorf("现有配置的 db 字段不是合法对象: %w", err)
		}
	}
	if db == nil {
		db = map[string]json.RawMessage{}
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		var old string
		decodeString(db, "dsn", &old)
		dsn = strings.TrimSpace(old)
	}
	if dsn == "" {
		return nil, fmt.Errorf("使用 MySQL 必须提供连接串 --db-dsn（形如 user:pass@tcp(127.0.0.1:3306)/dbname 或 mysql://user:pass@127.0.0.1:3306/dbname）")
	}
	converted, err := DSNFromURI(dsn)
	if err != nil {
		return nil, err
	}
	if err := setValue(db, "driver", "mysql"); err != nil {
		return nil, err
	}
	if err := setValue(db, "dsn", converted); err != nil {
		return nil, err
	}
	return db, nil
}

// DSNFromURI 把 mysql://user:pass@host:3306/dbname?charset=utf8mb4 形式的 URI
// 转换为 go-sql-driver/mysql 的 DSN；已是 DSN 形式时原样返回（缺 parseTime 时补齐）。
// 也接受 JDBC 写法 jdbc:mysql://...（常见于 Java 配置）。
func DSNFromURI(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("MySQL 连接串不能为空")
	}
	// 兼容 JDBC 写法：剥离 jdbc: 前缀后其余部分与 mysql:// URI 一致。
	if strings.HasPrefix(strings.ToLower(s), "jdbc:") {
		s = strings.TrimSpace(s[len("jdbc:"):])
	}
	if strings.HasPrefix(strings.ToLower(s), "mysql://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("解析 MySQL 连接串失败: %w", err)
		}
		if u.Host == "" {
			return "", fmt.Errorf("MySQL 连接串缺少主机地址：%s", s)
		}
		dbName := strings.TrimPrefix(u.Path, "/")
		if dbName == "" {
			return "", fmt.Errorf("MySQL 连接串缺少数据库名（形如 mysql://user:pass@127.0.0.1:3306/dbname）")
		}
		auth := ""
		if u.User != nil {
			name := u.User.Username()
			if pwd, ok := u.User.Password(); ok {
				auth = name + ":" + pwd + "@"
			} else if name != "" {
				auth = name + "@"
			}
		}
		dsn := fmt.Sprintf("%stcp(%s)/%s", auth, u.Host, dbName)
		q := normalizeMySQLQuery(u.Query())
		if q.Get("parseTime") == "" {
			q.Set("parseTime", "true")
		}
		if enc := q.Encode(); enc != "" {
			dsn += "?" + enc
		}
		return dsn, nil
	}
	if !strings.Contains(s, "@") {
		return "", fmt.Errorf("无法识别的 MySQL 连接串 %q：请使用 mysql://user:pass@host:3306/dbname、jdbc:mysql://... 或 user:pass@tcp(host:3306)/dbname", s)
	}
	return ensureParseTime(s), nil
}

// mysqlDSNParams 是 go-sql-driver/mysql 支持的 DSN 参数；其余参数会被驱动拒绝，
// 因此从 URI/JDBC 继承过来的参数需要过滤。
var mysqlDSNParams = map[string]bool{
	"allowAllFiles": true, "allowCleartextPasswords": true, "allowFallbackToPlaintext": true,
	"allowNativePasswords": true, "allowOldPasswords": true, "autocommit": true,
	"charset": true, "checkConnLiveness": true, "clientFoundRows": true, "collation": true,
	"columnsWithAlias": true, "compress": true, "connectionAttributes": true,
	"interpolateParams": true, "loc": true, "maxAllowedPacket": true, "multiStatements": true,
	"parseTime": true, "readTimeout": true, "rejectReadOnly": true, "serverPubKey": true,
	"timeout": true, "timeTruncate": true, "tls": true, "writeTimeout": true,
}

// normalizeMySQLQuery 把 URI/JDBC 的查询参数转换为驱动可识别的集合：
// 过滤驱动不支持的键，并映射常见 JDBC 参数（characterEncoding/serverTimezone/useSSL）。
func normalizeMySQLQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, vs := range q {
		if len(vs) == 0 || !mysqlDSNParams[k] {
			continue
		}
		out[k] = vs
	}
	if out.Get("charset") == "" {
		enc := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(q.Get("characterEncoding")), "-", ""))
		if enc == "utf8" || enc == "utf8mb4" {
			out.Set("charset", "utf8mb4")
		}
	}
	if out.Get("loc") == "" {
		if tz := strings.TrimSpace(q.Get("serverTimezone")); tz != "" && !strings.EqualFold(tz, "UTC") {
			out.Set("loc", tz)
		}
	}
	// useSSL=false 时驱动本来就不加密（无需转换）；true 时要求 TLS。
	if out.Get("tls") == "" && strings.EqualFold(strings.TrimSpace(q.Get("useSSL")), "true") {
		out.Set("tls", "true")
	}
	return out
}

// ensureParseTime 为 DSN 补齐 parseTime=true（服务按时间字段扫描数据）。
func ensureParseTime(dsn string) string {
	base, query, found := strings.Cut(dsn, "?")
	if !found || strings.TrimSpace(query) == "" {
		return base + "?parseTime=true"
	}
	for _, kv := range strings.Split(query, "&") {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(strings.TrimSpace(k), "parseTime") {
			return dsn
		}
	}
	return dsn + "&parseTime=true"
}

// MaskDSN 对 DSN 中的密码脱敏（user:pass@tcp(...) → user:***@tcp(...)）。
func MaskDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	colon := strings.Index(dsn, ":")
	if at <= 0 || colon <= 0 || colon > at {
		return dsn
	}
	return dsn[:colon+1] + "***" + dsn[at:]
}
