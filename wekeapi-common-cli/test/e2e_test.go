// Package test 端到端测试：编译当前平台二进制后以子进程方式运行，验证命令行行为与配置落盘。
package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var binPath string

// TestMain 编译一次当前平台二进制，供所有用例复用。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wekeapi-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "wekeapi"+exeSuffix())

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = ".." // 模块根目录
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "构建 wekeapi 失败: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// cliEnv 构造隔离的用户目录环境，避免读写真实 ~/.wekeapi。
func cliEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		name := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if name == "HOME" || name == "USERPROFILE" {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "HOME="+home, "USERPROFILE="+home)
}

// runCLI 以指定用户目录运行二进制，返回 stdout/stderr 与退出码。
func runCLI(t *testing.T, home string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = cliEnv(home)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err := cmd.Run()
	if err == nil {
		return out.String(), errOut.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.String(), errOut.String(), exitErr.ExitCode()
	}
	t.Fatalf("执行 wekeapi %v 失败: %v", args, err)
	return "", "", -1
}

func TestEnvCommand(t *testing.T) {
	stdout, stderr, code := runCLI(t, t.TempDir(), "env")
	if code != 0 {
		t.Fatalf("env 退出码 = %d, stderr = %s", code, stderr)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Env            string `json:"env"`
			BaseURL        string `json:"baseURL"`
			SiteURL        string `json:"siteURL"`
			ProxyDownloads []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"proxyDownloads"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("env 输出不是合法 JSON: %v（%s）", err, stdout)
	}
	if resp.Code != 0 || resp.Data.Env != "prod" {
		t.Errorf("env 响应不符: %+v", resp)
	}
	if resp.Data.BaseURL != "https://api.wekeapi.com/api" || resp.Data.SiteURL != "https://www.wekeapi.com" {
		t.Errorf("内置地址不符: baseURL=%q siteURL=%q", resp.Data.BaseURL, resp.Data.SiteURL)
	}
	if len(resp.Data.ProxyDownloads) != 5 {
		t.Errorf("proxyDownloads 数量 = %d, want 5（Windows x64/ARM64、macOS Intel/ARM、Linux）", len(resp.Data.ProxyDownloads))
	}
	for _, p := range resp.Data.ProxyDownloads {
		if p.Platform == "" || p.URL == "" {
			t.Errorf("proxyDownloads 项不完整: %+v", p)
		}
	}
}

func TestHelpOutput(t *testing.T) {
	stdout, stderr, code := runCLI(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatalf("--help 退出码 = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{"login", "contact", "message", "data"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--help 缺少命令 %q", want)
		}
	}
}

func TestMissingTokenExitCode(t *testing.T) {
	_, stderr, code := runCLI(t, t.TempDir(), "contact", "list")
	if code != 2 {
		t.Errorf("缺少 AccessToken 退出码 = %d, want 2（stderr=%s）", code, stderr)
	}
	if !strings.Contains(stderr, "AccessToken") {
		t.Errorf("stderr 未提示 AccessToken: %s", stderr)
	}
}

func TestInitWritesConfig(t *testing.T) {
	home := t.TempDir()
	if _, stderr, code := runCLI(t, home, "init",
		"--token", "tk_e2e",
		"--app-id", "app1",
		"--base-url", "https://api-e2e.example/api",
	); code != 0 {
		t.Fatalf("init 退出码 = %d, stderr = %s", code, stderr)
	}

	cfgPath := filepath.Join(home, ".wekeapi", "config.json")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("配置不是合法 JSON: %v（%s）", err, string(b))
	}
	if cfg["token"] != "tk_e2e" || cfg["appId"] != "app1" || cfg["baseURL"] != "https://api-e2e.example/api" {
		t.Errorf("配置内容不符: %+v", cfg)
	}

	// 只传 token 时保留已有 appId 与 baseURL。
	if _, stderr, code := runCLI(t, home, "init", "--token", "tk_new"); code != 0 {
		t.Fatalf("再次 init 退出码 = %d, stderr = %s", code, stderr)
	}
	b, err = os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读取更新后的配置失败: %v", err)
	}
	cfg = nil
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("配置不是合法 JSON: %v（%s）", err, string(b))
	}
	if cfg["token"] != "tk_new" || cfg["appId"] != "app1" || cfg["baseURL"] != "https://api-e2e.example/api" {
		t.Errorf("再次 init 后配置不符: %+v", cfg)
	}
}

func TestLoginQrMissingFlags(t *testing.T) {
	_, stderr, code := runCLI(t, t.TempDir(), "login", "qr", "--token", "tk")
	if code == 0 {
		t.Fatal("缺少必填参数时不应成功")
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("缺少必填参数时 stderr 应有提示")
	}
}
