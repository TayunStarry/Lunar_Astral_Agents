//go:build windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// 桌面操纵可观测性：逐角色 trace 日志。
// 记录每个角色（编纂者/启动者/视觉/UIA/规划/操作/书记）「读到什么、想了什么、决定做什么、做了什么」，
// 便于复盘。

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
	autoTraceDir = filepath.Join(filepath.Dir(execPath), config.LocalDir, "logs")
	_ = os.MkdirAll(autoTraceDir, 0755)
}

// traceAuto 写一条桌面操纵 trace 日志（时间 + 说明文字）
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
