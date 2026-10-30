//go:build !windows

package msgservice

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// processAlive 判断进程是否存在（0 信号探测；EPERM 说明进程存在但无权限）。
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// terminateProcess 发送 SIGTERM；force 为 true 时发送 SIGKILL。
func terminateProcess(pid int, force bool) error {
	if pid <= 0 {
		return fmt.Errorf("无效的进程号 %d", pid)
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("终止进程 %d 失败: %w", pid, err)
	}
	return nil
}

// startAttrs 让服务进程脱离当前会话，CLI 退出后仍继续运行。
func startAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

var ssPIDPattern = regexp.MustCompile(`pid=(\d+)`)

// findPIDByPort 查找监听指定 TCP 端口的进程（lsof 优先，ss 兜底）。
func findPIDByPort(port int) (int, bool) {
	out, err := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && pid > 0 {
				return pid, true
			}
		}
	}
	out, err = exec.Command("ss", "-lptnH", fmt.Sprintf("sport = :%d", port)).Output()
	if err == nil {
		if m := ssPIDPattern.FindStringSubmatch(string(out)); len(m) == 2 {
			if pid, err := strconv.Atoi(m[1]); err == nil && pid > 0 {
				return pid, true
			}
		}
	}
	return 0, false
}

// processImageName 返回进程可执行文件名。
func processImageName(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
