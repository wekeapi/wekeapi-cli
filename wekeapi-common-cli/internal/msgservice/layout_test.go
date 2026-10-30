package msgservice

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLayoutFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)

	layout, err := ResolveLayout("", "")
	if err != nil {
		t.Fatalf("ResolveLayout: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if layout.Home != abs {
		t.Fatalf("Home 应为 %s，实际 %s", abs, layout.Home)
	}
	if layout.ConfigPath != filepath.Join(abs, ConfigFileName) {
		t.Fatalf("ConfigPath 不符: %s", layout.ConfigPath)
	}
	if layout.PIDPath != filepath.Join(abs, PIDFileName) {
		t.Fatalf("PIDPath 不符: %s", layout.PIDPath)
	}
	if layout.LogPath != filepath.Join(abs, LogFileName) {
		t.Fatalf("LogPath 不符: %s", layout.LogPath)
	}
}

func TestResolveLayoutDefaultHome(t *testing.T) {
	t.Setenv(EnvHome, "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取用户主目录")
	}

	layout, err := ResolveLayout("", "")
	if err != nil {
		t.Fatalf("ResolveLayout: %v", err)
	}
	want := filepath.Join(home, ".wekeapi", "msg-sync")
	if layout.Home != want {
		t.Fatalf("默认服务目录应为 %s，实际 %s", want, layout.Home)
	}
}

func TestResolveLayoutPriority(t *testing.T) {
	explicit := t.TempDir()
	envDir := t.TempDir()
	fallback := t.TempDir()
	t.Setenv(EnvHome, envDir)

	check := func(got, want string) {
		t.Helper()
		abs, _ := filepath.Abs(want)
		if got != abs {
			t.Fatalf("want %s, got %s", abs, got)
		}
	}

	layout, err := ResolveLayout(explicit, fallback)
	if err != nil {
		t.Fatal(err)
	}
	check(layout.Home, explicit)

	layout, err = ResolveLayout("", fallback)
	if err != nil {
		t.Fatal(err)
	}
	check(layout.Home, envDir)

	t.Setenv(EnvHome, "")
	layout, err = ResolveLayout("", fallback)
	if err != nil {
		t.Fatal(err)
	}
	check(layout.Home, fallback)
}

func TestBinaryPathLocal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	layout, err := ResolveLayout("", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := layout.BinaryPath(); err == nil {
		t.Fatalf("目录为空时应找不到服务可执行文件")
	}

	// dist 子目录下的平台命名二进制也应被识别。
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dist, BinaryName+exeSuffix())
	if err := os.WriteFile(bin, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := layout.BinaryPath()
	if err != nil {
		t.Fatalf("BinaryPath: %v", err)
	}
	if got != bin {
		t.Fatalf("BinaryPath 应为 %s，实际 %s", bin, got)
	}
}

func TestAbsPathTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取用户主目录")
	}
	got, err := absPath("~/.wekeapi/msg-sync")
	if err != nil {
		t.Fatalf("absPath: %v", err)
	}
	want := filepath.Join(home, ".wekeapi", "msg-sync")
	if got != want {
		t.Fatalf("absPath(~) 应为 %s，实际 %s", want, got)
	}
}
