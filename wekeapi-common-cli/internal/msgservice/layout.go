package msgservice

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// ConfigFileName 服务配置文件名。
	ConfigFileName = "wekeapi-msg-sync.json"
	// PIDFileName 服务 PID 文件名。
	PIDFileName = "wekeapi-msg-sync.pid"
	// LogFileName CLI 拉起服务时的日志文件名。
	LogFileName = "wekeapi-msg-sync.log"
	// BinaryName 服务可执行文件名（不含平台后缀）。
	BinaryName = "wekeapi-msg-sync"
	// EnvHome 指定服务目录的环境变量（优先级最高）。
	EnvHome = "WEKEAPI_MSG_SYNC_HOME"
)

// Layout 服务运行目录布局。
type Layout struct {
	Home       string // 服务目录（配置、PID、日志所在）
	ConfigPath string
	PIDPath    string
	LogPath    string
}

// ResolveLayout 解析服务目录（配置、日志、PID、SQLite 均在其中）：
//  1. explicit（--msg-sync-dir 等显式目录）；
//  2. 环境变量 WEKEAPI_MSG_SYNC_HOME；
//  3. fallback（主 CLI 配置中的 msgSyncDir）；
//  4. 默认 ~/.wekeapi/msg-sync（不再从当前目录向上查找服务目录）。
func ResolveLayout(explicit, fallback string) (*Layout, error) {
	home, err := resolveHome(explicit, fallback)
	if err != nil {
		return nil, err
	}
	return &Layout{
		Home:       home,
		ConfigPath: filepath.Join(home, ConfigFileName),
		PIDPath:    filepath.Join(home, PIDFileName),
		LogPath:    filepath.Join(home, LogFileName),
	}, nil
}

func resolveHome(explicit, fallback string) (string, error) {
	if v := strings.TrimSpace(explicit); v != "" {
		return absPath(v)
	}
	if v := strings.TrimSpace(os.Getenv(EnvHome)); v != "" {
		return absPath(v)
	}
	if v := strings.TrimSpace(fallback); v != "" {
		return absPath(v)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("未获取到用户主目录，无法定位默认服务目录 ~/.wekeapi/msg-sync: %w", err)
	}
	return filepath.Join(home, ".wekeapi", "msg-sync"), nil
}

// findBinary 查找服务可执行文件：服务目录（含 dist/bin）> 主 CLI 同目录 > PATH。
// skill 场景下主 CLI 与服务二进制同放在 assets/bin，因此需要看主 CLI 同目录。
func findBinary(dir string) (string, error) {
	if p, ok := findLocalBinary(dir); ok {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		if p, ok := findLocalBinary(filepath.Dir(exe)); ok {
			return p, nil
		}
	}
	if p, err := exec.LookPath(BinaryName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("未找到 %s 可执行文件（服务目录 %s、主 CLI 同目录与 PATH 均未找到）", BinaryName, dir)
}

// findLocalBinary 仅在给定目录（含 dist/bin 子目录）内查找当前平台的可执行文件。
func findLocalBinary(dir string) (string, bool) {
	names := []string{
		BinaryName + exeSuffix(),
		fmt.Sprintf("%s-%s-%s%s", BinaryName, runtime.GOOS, runtime.GOARCH, exeSuffix()),
	}
	for _, sub := range []string{"", "dist", "bin"} {
		for _, name := range names {
			p := filepath.Join(dir, sub, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
		}
	}
	return "", false
}

// BinaryPath 返回服务可执行文件路径。
func (l *Layout) BinaryPath() (string, error) {
	return findBinary(l.Home)
}

// Config 读取服务配置。
func (l *Layout) Config() (*ServiceConfig, error) {
	return LoadConfig(l.ConfigPath)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func absPath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), string(os.PathSeparator)))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("解析路径 %s 失败: %w", p, err)
	}
	return abs, nil
}
