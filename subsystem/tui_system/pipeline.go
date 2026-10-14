package TuiSystem

import (
	"bufio"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// 月华 JS 日志的模块/子标签（与 engine 绑定自定义 console 时写入的标签保持一致）
const (
	lunarModule = "LunarCore"
	lunarSubJS  = "JavaScript"
	// ltp9 沙箱日志的模块标签（engine/ltp9/variable.go ServiceName）
	ltp9Module = "StarLTP"
	// ltp9 沙箱 console 输出携带的标记（binder.go consolePrint）
	ltp9ConsoleMark = "[console."
)

// Install 装配统一日志处理管线（幂等）：
//  1. 以管道接管 os.Stdout/os.Stderr，后台逐行扫描送入原始日志缓冲；
//  2. 注册 LoggerGeneral 日志钩子，按模块/级别路由到各日志视图缓冲。
//
// 必须在宿主程序启动最早期（任何子系统装配前）调用，确保启动期日志不逃逸到终端。
func Install() {
	pipelineOnce.Do(func() {
		// ==== stdout/stderr 管道接管 ====
		stdoutR, stdoutW, err := os.Pipe()
		if err == nil {
			realStdout = os.Stdout
			realStderr = os.Stderr
			os.Stdout = stdoutW
			os.Stderr = stdoutW
			LoggerGeneral.SetOutput(stdoutW)
			intercepted = true
			go scanPipeLines(stdoutR, SourceRaw)
		}

		// ==== LoggerGeneral 结构化日志钩子 ====
		LoggerGeneral.SetLogHook(routeLogRecord)

		// ==== 运行状态采样器 ====
		initSampler()
		go samplerLoop()
	})
}

// GetRealStdout 返回原始终端输出（TUI 渲染目标；未接管时为 os.Stdout）
func GetRealStdout() interface{ Write([]byte) (int, error) } {
	if realStdout != nil {
		return realStdout
	}
	return os.Stdout
}

// scanPipeLines 逐行扫描管道内容并送入原始日志缓冲
func scanPipeLines(r *os.File, source LogSource) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "stdout",
			Level:  "RAW",
			Text:   line,
		}, SourceRaw)
	}
}

// routeLogRecord LoggerGeneral 钩子：按模块/级别分类路由
func routeLogRecord(level, module, sub, msg string) {
	line := LogLine{
		Time:   time.Now(),
		Source: "log",
		Level:  level,
		Module: module,
		Sub:    sub,
		Text:   msg,
	}
	// 原始日志视图接收全量
	pushLog(line, SourceRaw)

	switch {
	// 月华 JS 层日志
	case module == lunarModule && sub == lunarSubJS:
		pushLog(line, SourceLunarJS)
	// ltp9 沙箱 JS 层日志（console 输出带 [console.*] 标记）
	case module == ltp9Module && strings.Contains(msg, ltp9ConsoleMark):
		pushLog(line, SourceLTP9JS)
	}

	// Go 警告与错误（独立于上面分支，原始日志不受影响）
	if level == "WARN" || level == "ERROR" || level == "FATAL" {
		pushLog(line, SourceGoWarn)
	}
}

// pushLog 追加一条日志到指定来源缓冲（环形截断 + 唤醒 TUI）
func pushLog(line LogLine, source LogSource) {
	storeMu.Lock()
	store := &logStores[source]
	*store = append(*store, line)
	if len(*store) > logStoreCap {
		*store = (*store)[len(*store)-logStoreCap:]
	}
	storeMu.Unlock()
	wakeTUI()
}

// PushTaskLog 功能子包（代理/打包/端口释放/更新包）日志统一出口：
// 同时并入原始日志视图与功能任务回显缓冲。
func PushTaskLog(msg string) {
	line := LogLine{
		Time:   time.Now(),
		Source: "task",
		Level:  classifyTaskLevel(msg),
		Text:   msg,
	}
	pushLog(line, SourceRaw)
	pushLog(line, SourceTask)
}

// classifyTaskLevel 按文本标记推断任务日志级别（沿用 environment_repair 的 [ERROR]/[WARN] 前缀约定）
func classifyTaskLevel(msg string) string {
	switch {
	case strings.Contains(msg, "[ERROR]"), strings.Contains(msg, "✗"):
		return "ERROR"
	case strings.Contains(msg, "[WARN]"):
		return "WARN"
	default:
		return "INFO"
	}
}

// GetLogs 返回指定来源的日志缓冲副本（TUI 视图渲染数据源）
func GetLogs(source LogSource) []LogLine {
	storeMu.RLock()
	defer storeMu.RUnlock()
	out := make([]LogLine, len(logStores[source]))
	copy(out, logStores[source])
	return out
}

// ClearLogs 清空指定来源的日志缓冲
func ClearLogs(source LogSource) {
	storeMu.Lock()
	logStores[source] = nil
	storeMu.Unlock()
	wakeTUI()
}

// wakeTUI 非阻塞唤醒 TUI 刷新（缓冲 1，重复通知合并）
func wakeTUI() {
	select {
	case notifyCh <- struct{}{}:
	default:
	}
}
