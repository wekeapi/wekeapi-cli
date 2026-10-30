package msgservice

import (
	"encoding/json"
	"fmt"
	"os"
)

// pidInfo 与 wekeapi-msg-sync 写入的 wekeapi-msg-sync.pid 对应。
type pidInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Config    string `json:"config"`
	StartedAt string `json:"started_at"`
}

func readPID(path string) (*pidInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info pidInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("PID 文件 %s 不是合法 JSON: %w", path, err)
	}
	if info.PID <= 0 {
		return nil, fmt.Errorf("PID 文件 %s 中的 pid 无效", path)
	}
	return &info, nil
}

func removePID(path string) {
	_ = os.Remove(path)
}

// sameConfig 判断 PID 文件是否属于指定配置文件（空值视为匹配）。
func (i *pidInfo) sameConfig(configPath string) bool {
	if i.Config == "" {
		return true
	}
	return i.Config == configPath
}
