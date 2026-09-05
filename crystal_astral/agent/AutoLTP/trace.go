//go:build windows

package AutoLTP

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"
)

// Auto-LTP 可观测性：逐角色 trace 日志 + 配图归档。
// 记录每个角色（编纂者/启动者/视觉/UIA/规划/操作/书记）「读到什么、想了什么、决定做什么、做了什么」，
// 配合截图归档到 images/moment，便于复盘。

var (
	autoTraceOnce sync.Once
	autoTraceDir  string
	autoTraceMu   sync.Mutex
)

// autoTraceInitPath 惰性解析日志目录（exeDir + LocalDir，运行于项目根即 d:\Lunar_Astral_Agents\local_data\logs）
func autoTraceInitPath() {
	execPath, err := os.Executable()
	if err != nil {
		autoTraceDir = "."
		return
	}
	autoTraceDir = filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "logs")
	_ = os.MkdirAll(autoTraceDir, 0755)
}

// autoShotDirPath 惰性解析截图目录（images/moment）
func autoShotDirPath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "images", "moment")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// traceAuto 写一条 Auto-LTP trace 日志（时间 + 说明文字）
func traceAuto(format string, args ...any) {
	autoTraceOnce.Do(autoTraceInitPath)
	line := time.Now().Format("15:04:05.000") + " | " + fmt.Sprintf(format, args...)
	autoTraceMu.Lock()
	defer autoTraceMu.Unlock()
	f, err := os.OpenFile(filepath.Join(autoTraceDir, "auto_ltp_trace.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line + "\n")
}

// saveAutoShot 把当前截图归档到 images/moment 并返回路径（供 trace 关联）
func saveAutoShot(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	dir := autoShotDirPath()
	name := fmt.Sprintf("auto_shot_%s.png", time.Now().Format("20060102_150405.000"))
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		return ""
	}
	return p
}
