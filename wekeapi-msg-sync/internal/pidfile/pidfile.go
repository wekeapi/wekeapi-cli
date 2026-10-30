// Package pidfile 维护服务进程的 PID 文件。
// CLI（wekeapi data start/stop/sync）依赖该文件找到已启动的服务实例并重启它。
package pidfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName PID 文件名（与配置文件同目录）。
const FileName = "wekeapi-msg-sync.pid"

// Info PID 文件内容。
type Info struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Config    string `json:"config"`
	StartedAt string `json:"started_at"`
}

// PathFor 返回配置文件对应的 PID 文件路径（与配置文件同目录）。
func PathFor(configPath string) string {
	abs := configPath
	if p, err := filepath.Abs(configPath); err == nil {
		abs = p
	}
	return filepath.Join(filepath.Dir(abs), FileName)
}

// Write 写入 PID 文件（0600）。
func Write(path string, info Info) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 PID 文件失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建 PID 文件目录失败: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("写入 PID 文件 %s 失败: %w", path, err)
	}
	return nil
}

// Remove 删除 PID 文件；文件不存在时不报错。
func Remove(path string) {
	_ = os.Remove(path)
}

// Read 读取 PID 文件。
func Read(path string) (*Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("PID 文件 %s 不是合法 JSON: %w", path, err)
	}
	if info.PID <= 0 {
		return nil, fmt.Errorf("PID 文件 %s 中的 pid 无效", path)
	}
	return &info, nil
}

// SameConfig 判断 PID 文件是否属于指定配置文件（空值视为匹配，兼容早期版本）。
func (i *Info) SameConfig(configPath string) bool {
	if strings.TrimSpace(i.Config) == "" {
		return true
	}
	a, err1 := filepath.Abs(i.Config)
	b, err2 := filepath.Abs(configPath)
	if err1 != nil || err2 != nil {
		return filepath.Clean(i.Config) == filepath.Clean(configPath)
	}
	return a == b
}
