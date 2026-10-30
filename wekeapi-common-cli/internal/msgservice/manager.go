package msgservice

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EnsureOptions 登录成功后同步配置并确保服务运行（已在运行且凭证无变化时保持不动）的选项。
type EnsureOptions struct {
	Settings Settings
	Logf     func(format string, args ...any)
	// AskDB 仅当服务配置尚不存在（首次创建 wekeapi-msg-sync.json）时调用，
	// 由外层（交互终端）询问数据库类型：返回 nil 表示沿用默认配置（SQLite），
	// 返回错误则中止本次服务初始化；配置已存在时不会调用。
	AskDB func() (*ConfigureOptions, error)
}

// State 服务管理结果（输出为 JSON）。
type State struct {
	Action        string `json:"action"` // started / restarted / unchanged / stopped
	Running       bool   `json:"running"`
	PID           int    `json:"pid,omitempty"`
	Port          int    `json:"port"`
	ConfigPath    string `json:"configPath"`
	BinaryPath    string `json:"binaryPath,omitempty"`
	LogPath       string `json:"logPath,omitempty"`
	ConfigChanged bool   `json:"configChanged"`
	AppID         string `json:"appId,omitempty"`
	WxID          string `json:"wxid,omitempty"`
}

// Ensure 把主 CLI 凭证写入服务配置，并确保服务运行：
// 已启动且配置无变化时保持不动（不重复启动）；仅当服务未运行或凭证确有变化时才启动/重启。
func Ensure(ctx context.Context, layout *Layout, opts EnsureOptions) (*State, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := validateSettings(opts.Settings); err != nil {
		return nil, err
	}
	if err := applyFirstRunDatabase(ctx, layout, opts, logf); err != nil {
		return nil, err
	}

	changed, cfg, err := WriteConfig(layout.ConfigPath, opts.Settings)
	if err != nil {
		return nil, err
	}
	st := &State{
		Port:          cfg.Port,
		ConfigPath:    layout.ConfigPath,
		LogPath:       layout.LogPath,
		ConfigChanged: changed,
		AppID:         cfg.AppID,
		WxID:          cfg.WxID,
	}

	running, pid := probe(ctx, layout, cfg.Port)
	if running && !changed {
		st.Action, st.Running, st.PID = "unchanged", true, pid
		logf("数据同步服务已在运行（pid=%d，端口 %d），配置无变化，保持不动", pid, cfg.Port)
		return st, nil
	}
	if running {
		// 仅凭证/配置确实变化时才重启：新 token/appId/wxid 只能通过重启生效，
		// 否则保持现有进程，避免打断可能正在进行中的全量同步。
		logf("数据同步服务配置已变化，需重启后生效")
	}

	// 先确认可执行文件存在，避免停掉正在运行的服务后才发现无法启动。
	bin, err := layout.BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("%w；可通过环境变量 %s 指定服务所在目录", err, EnvHome)
	}
	st.BinaryPath = bin
	if running {
		stopped, err := stopRunning(ctx, layout, cfg.Port, pid)
		if err != nil {
			return nil, err
		}
		logf("已停止数据同步服务（pid=%d），准备使用最新配置重启", stopped)
		st.Action = "restarted"
	} else {
		st.Action = "started"
	}

	newPID, err := startService(ctx, layout, cfg.Port, logf)
	if err != nil {
		return nil, err
	}
	st.Running, st.PID = true, newPID
	logf("数据同步服务已%s（pid=%d，端口 %d，配置 %s）", actionText(st.Action), newPID, cfg.Port, layout.ConfigPath)
	return st, nil
}

// applyFirstRunDatabase 首次创建服务配置（wekeapi-msg-sync.json 不存在）时确定数据库：
// 交互终端由 AskDB 询问（默认 SQLite），非交互场景使用默认 SQLite 并提示修改方式。
// 配置已存在时不改动，保证后续登录/在线检测不会覆盖用户已选好的数据库。
func applyFirstRunDatabase(ctx context.Context, layout *Layout, opts EnsureOptions, logf func(string, ...any)) error {
	if _, err := os.Stat(layout.ConfigPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取数据同步服务配置 %s 失败: %w", layout.ConfigPath, err)
	}
	if opts.AskDB == nil {
		logf("未检测到数据同步服务配置，按默认使用 SQLite（可用 wekeapi data config 修改数据库与端口）")
		return nil
	}
	choice, err := opts.AskDB()
	if err != nil {
		return err
	}
	if choice == nil {
		return nil
	}
	if _, err := Configure(ctx, layout, *choice); err != nil {
		return err
	}
	return nil
}

// Start 启动服务；已运行时不重启。
func Start(ctx context.Context, layout *Layout) (*State, error) {
	cfg, err := loadValidConfig(layout)
	if err != nil {
		return nil, err
	}
	st := &State{Port: cfg.Port, ConfigPath: layout.ConfigPath, LogPath: layout.LogPath, AppID: cfg.AppID, WxID: cfg.WxID}
	running, pid := probe(ctx, layout, cfg.Port)
	if running {
		st.Action, st.Running, st.PID = "unchanged", true, pid
		return st, nil
	}
	bin, err := layout.BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("%w；可通过环境变量 %s 指定服务所在目录", err, EnvHome)
	}
	st.BinaryPath = bin
	newPID, err := startService(ctx, layout, cfg.Port, nil)
	if err != nil {
		return nil, err
	}
	st.Action, st.Running, st.PID = "started", true, newPID
	return st, nil
}

// Restart 关闭已运行的服务并重新启动（重新启动会触发一次全量同步）。
func Restart(ctx context.Context, layout *Layout) (*State, error) {
	cfg, err := loadValidConfig(layout)
	if err != nil {
		return nil, err
	}
	st := &State{Port: cfg.Port, ConfigPath: layout.ConfigPath, LogPath: layout.LogPath, AppID: cfg.AppID, WxID: cfg.WxID}

	// 先确认可执行文件存在，避免停掉正在运行的服务后才发现无法启动。
	bin, err := layout.BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("%w；可通过环境变量 %s 指定服务所在目录", err, EnvHome)
	}
	st.BinaryPath = bin

	running, pid := probe(ctx, layout, cfg.Port)
	if running {
		if _, err := stopRunning(ctx, layout, cfg.Port, pid); err != nil {
			return nil, err
		}
		st.Action = "restarted"
	} else {
		st.Action = "started"
	}
	newPID, err := startService(ctx, layout, cfg.Port, nil)
	if err != nil {
		return nil, err
	}
	st.Running, st.PID = true, newPID
	return st, nil
}

// Stop 关闭服务。
func Stop(ctx context.Context, layout *Layout) (*State, error) {
	port := servicePort(layout)
	st := &State{Action: "stopped", Port: port, ConfigPath: layout.ConfigPath, LogPath: layout.LogPath}
	running, pid := probe(ctx, layout, port)
	if !running {
		removePID(layout.PIDPath)
		st.PID = 0
		return st, nil
	}
	stopped, err := stopRunning(ctx, layout, port, pid)
	if err != nil {
		return nil, err
	}
	st.PID = stopped
	st.Running = false
	return st, nil
}

// probe 判断服务是否在运行：优先 PID 文件，其次健康检查。
func probe(ctx context.Context, layout *Layout, port int) (bool, int) {
	if info, err := readPID(layout.PIDPath); err == nil {
		if info.sameConfig(layout.ConfigPath) && processAlive(info.PID) {
			return true, info.PID
		}
	}
	if Healthy(ctx, port) {
		return true, 0
	}
	return false, 0
}

func stopRunning(ctx context.Context, layout *Layout, port, pid int) (int, error) {
	if pid <= 0 {
		found, ok := findPIDByPort(port)
		if !ok {
			if Healthy(ctx, port) {
				return 0, fmt.Errorf("端口 %d 上的数据同步服务正在运行，但缺少 PID 文件且无法定位进程，请手动停止后重试", port)
			}
			removePID(layout.PIDPath)
			return 0, nil
		}
		name := processImageName(found)
		if !strings.Contains(strings.ToLower(name), strings.ToLower(BinaryName)) {
			return 0, fmt.Errorf("端口 %d 被进程 %d（%s）占用，且不是 %s，请更换端口或手动停止该进程",
				port, found, name, BinaryName)
		}
		pid = found
	}

	_ = terminateProcess(pid, false)
	if waitGone(ctx, pid, port, 8*time.Second) {
		removePID(layout.PIDPath)
		return pid, nil
	}
	_ = terminateProcess(pid, true)
	if waitGone(ctx, pid, port, 3*time.Second) {
		removePID(layout.PIDPath)
		return pid, nil
	}
	return 0, fmt.Errorf("数据同步服务进程 %d 未能在超时内退出，请手动检查", pid)
}

func waitGone(ctx context.Context, pid, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if (pid <= 0 || !processAlive(pid)) && !Healthy(ctx, port) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func startService(ctx context.Context, layout *Layout, port int, logf func(string, ...any)) (int, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	bin, err := layout.BinaryPath()
	if err != nil {
		return 0, fmt.Errorf("%w；可通过环境变量 %s 指定服务所在目录", err, EnvHome)
	}
	if err := os.MkdirAll(layout.Home, 0o700); err != nil {
		return 0, fmt.Errorf("创建数据同步服务目录 %s 失败: %w%s", layout.Home, err, pathErrHint(err))
	}
	logFile, err := os.OpenFile(layout.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("打开服务日志 %s 失败: %w%s", layout.LogPath, err, pathErrHint(err))
	}

	cmd := exec.Command(bin, "run", "--config", layout.ConfigPath)
	cmd.Dir = layout.Home
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = startAttrs()
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("启动数据同步服务失败: %w", err)
	}
	pid := cmd.Process.Pid
	logf("正在启动数据同步服务：%s run --config %s（pid=%d）", bin, layout.ConfigPath, pid)

	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		logFile.Close()
	}()

	deadline := time.Now().Add(20 * time.Second)
	for {
		if Healthy(ctx, port) {
			return pid, nil
		}
		select {
		case err := <-exited:
			return 0, fmt.Errorf("数据同步服务启动失败（进程已退出: %v）\n日志：%s\n%s",
				err, layout.LogPath, tailLog(layout.LogPath, 20))
		default:
		}
		if time.Now().After(deadline) {
			_ = terminateProcess(pid, true)
			return 0, fmt.Errorf("数据同步服务启动后 20 秒内未就绪（端口 %d）\n日志：%s\n%s",
				port, layout.LogPath, tailLog(layout.LogPath, 20))
		}
		select {
		case <-ctx.Done():
			_ = terminateProcess(pid, true)
			return 0, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func loadValidConfig(layout *Layout) (*ServiceConfig, error) {
	cfg, err := layout.Config()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.WxID) == "" {
		return nil, fmt.Errorf("数据同步服务配置不完整（%s）：token/appId/wxid 均不能为空，请先登录一次", layout.ConfigPath)
	}
	return cfg, nil
}

func validateSettings(s Settings) error {
	var missing []string
	if strings.TrimSpace(s.Token) == "" {
		missing = append(missing, "token")
	}
	if strings.TrimSpace(s.AppID) == "" {
		missing = append(missing, "appId")
	}
	if strings.TrimSpace(s.WxID) == "" {
		missing = append(missing, "wxid")
	}
	if strings.TrimSpace(s.BaseURL) == "" {
		missing = append(missing, "baseURL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("无法启动数据同步服务：缺少 %s（请先完成登录）", strings.Join(missing, "/"))
	}
	return nil
}

func servicePort(layout *Layout) int {
	if info, err := readPID(layout.PIDPath); err == nil && info.Port > 0 {
		return info.Port
	}
	if cfg, err := LoadConfig(layout.ConfigPath); err == nil && cfg.Port > 0 {
		return cfg.Port
	}
	return DefaultPort
}

func actionText(action string) string {
	switch action {
	case "restarted":
		return "重启"
	case "unchanged":
		return "保持运行"
	default:
		return "启动"
	}
}

// tailLog 返回日志文件末尾 n 行（最多读取末尾 64KB）。
func tailLog(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	const maxTail = 64 << 10
	start := int64(0)
	if info.Size() > maxTail {
		start = info.Size() - maxTail
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
