// Package config 负责 wekeapi-msg-sync 配置文件的加载、默认值填充与校验。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 默认值。命令行 --port 与配置文件中的 port 均未提供时使用 DefaultPort。
const (
	DefaultFileName               = "wekeapi-msg-sync.json"
	DefaultBaseURL                = "https://api.wekeapi.com/api"
	DefaultPort                   = 3006
	DefaultDBPath                 = "./wekeapi_msg_sync.db"
	DefaultMaxOpenConns           = 10
	DefaultMaxIdleConns           = 5
	DefaultConnMaxLifetimeSeconds = 3600
)

// DBConfig MySQL 连接配置（与 db_path 互斥）。
type DBConfig struct {
	Driver                 string `json:"driver"`                    // 目前仅支持 mysql
	DSN                    string `json:"dsn"`                       // go-sql-driver/mysql DSN
	MaxOpenConns           int    `json:"max_open_conns"`            // 默认 10
	MaxIdleConns           int    `json:"max_idle_conns"`            // 默认 5
	ConnMaxLifetimeSeconds int    `json:"conn_max_lifetime_seconds"` // 默认 3600
}

// Config 服务配置。
type Config struct {
	Token   string    `json:"token"`
	AppID   string    `json:"appId"`
	BaseURL string    `json:"baseURL"`
	WxID    string    `json:"wxid"`
	Port    int       `json:"port"`
	DBPath  string    `json:"db_path"`
	DB      *DBConfig `json:"db,omitempty"`

	// Path 实际加载的配置文件路径（不参与 JSON 序列化）。
	Path string `json:"-"`
}

// DefaultHome 返回服务默认目录：~/.wekeapi/msg-sync，配置、PID、日志与 SQLite 数据库都放在这里。
func DefaultHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".wekeapi", "msg-sync")
}

// DefaultPath 返回默认配置文件路径：~/.wekeapi/msg-sync/wekeapi-msg-sync.json。
func DefaultPath() string {
	return filepath.Join(DefaultHome(), DefaultFileName)
}

// ResolvedDBPath 返回 SQLite 数据库文件的实际路径：
// 相对路径按配置文件所在目录解析，保证配置与数据总在同一目录，与启动时的当前目录无关。
func (c *Config) ResolvedDBPath() string {
	if c.DB != nil || strings.TrimSpace(c.DBPath) == "" {
		return ""
	}
	if filepath.IsAbs(c.DBPath) || c.Path == "" {
		return c.DBPath
	}
	base := filepath.Dir(c.Path)
	if base == "." || base == "" {
		return c.DBPath
	}
	return filepath.Join(base, c.DBPath)
}

// Load 读取配置文件，填充默认值并校验。
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("配置文件 %s 不存在：请先执行 wekeapi login（登录成功后会自动生成并启动服务），或复制 wekeapi-msg-sync.example.json 为该文件并填写后重试", path)
		}
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("配置文件 %s 不是合法 JSON: %w", path, err)
	}
	cfg.Path = path
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置文件 %s 校验失败: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	c.Token = strings.TrimSpace(c.Token)
	c.AppID = strings.TrimSpace(c.AppID)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.WxID = strings.TrimSpace(c.WxID)
	c.DBPath = strings.TrimSpace(c.DBPath)

	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.DB == nil && c.DBPath == "" {
		c.DBPath = DefaultDBPath
	}
	if c.DB != nil {
		c.DB.Driver = strings.ToLower(strings.TrimSpace(c.DB.Driver))
		c.DB.DSN = strings.TrimSpace(c.DB.DSN)
		if c.DB.MaxOpenConns <= 0 {
			c.DB.MaxOpenConns = DefaultMaxOpenConns
		}
		if c.DB.MaxIdleConns <= 0 {
			c.DB.MaxIdleConns = DefaultMaxIdleConns
		}
		if c.DB.ConnMaxLifetimeSeconds <= 0 {
			c.DB.ConnMaxLifetimeSeconds = DefaultConnMaxLifetimeSeconds
		}
	}
}

// Validate 校验配置，返回首个错误。
func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token 不能为空")
	}
	if c.AppID == "" {
		return fmt.Errorf("appId 不能为空")
	}
	if c.WxID == "" {
		return fmt.Errorf("wxid 不能为空（用于区分消息收发与拆分群消息发送人，可先登录后从平台获取）")
	}
	if !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("baseURL 必须以 http:// 或 https:// 开头，当前为 %q", c.BaseURL)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port 必须在 1-65535 之间，当前为 %d", c.Port)
	}
	if c.DB != nil && c.DBPath != "" {
		return fmt.Errorf("db_path 与 db 互斥：使用 SQLite 请删除 db 配置，使用 MySQL 请删除 db_path 配置")
	}
	if c.DB != nil {
		if c.DB.Driver == "" {
			return fmt.Errorf("db.driver 不能为空（目前仅支持 mysql）")
		}
		if c.DB.Driver != "mysql" {
			return fmt.Errorf("db.driver 仅支持 mysql，当前为 %q；使用 SQLite 请删除 db 配置并改用 db_path", c.DB.Driver)
		}
		if c.DB.DSN == "" {
			return fmt.Errorf("db.dsn 不能为空")
		}
	}
	return nil
}

// Addr 返回 HTTP 监听地址。
func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

// DBKind 返回数据库类型名（sqlite / mysql）。
func (c *Config) DBKind() string {
	if c.DB != nil {
		return c.DB.Driver
	}
	return "sqlite"
}

// MaskToken 对 token 脱敏，仅保留首尾各 4 位。
func MaskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
