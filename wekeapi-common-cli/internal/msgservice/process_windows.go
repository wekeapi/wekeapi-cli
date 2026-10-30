//go:build windows

package msgservice

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// processAlive 判断进程是否存在且未退出。
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// stillActive Win32 STILL_ACTIVE（STATUS_PENDING）。
const stillActive = 259

// terminateProcess 结束进程。Windows 下无 SIGTERM，统一使用 TerminateProcess；
// 服务端 SQLite 使用 WAL + 事务，强制退出不会损坏数据。
func terminateProcess(pid int, force bool) error {
	if pid <= 0 {
		return fmt.Errorf("无效的进程号 %d", pid)
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("打开进程 %d 失败: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("终止进程 %d 失败: %w", pid, err)
	}
	_, _ = windows.WaitForSingleObject(h, 5000)
	return nil
}

// startAttrs 让服务进程脱离当前控制台，CLI 退出/关闭终端后仍继续运行。
func startAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}

// findPIDByPort 通过 netstat 查找监听指定端口的进程。
func findPIDByPort(port int) (int, bool) {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return 0, false
	}
	needle := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.HasSuffix(fields[1], needle) || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if pid, err := strconv.Atoi(fields[4]); err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

// processImageName 通过 tasklist 返回进程映像名。
func processImageName(pid int) string {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if line == "" || strings.HasPrefix(strings.ToUpper(line), "INFO:") {
		return ""
	}
	if i := strings.Index(line, `","`); i > 0 {
		return strings.Trim(line[:i], `"`)
	}
	return ""
}
