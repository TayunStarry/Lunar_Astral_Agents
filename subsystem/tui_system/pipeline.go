package TuiSystem

import (
	"bufio"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	// 绘图模块的日志模块标签（multimodal_analysis/module）
	drawModule = "MultimodalAnalysis"
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
			// 渲染目标：继承来的 stdout 未必可用（被重定向 / NUL / 只写句柄），
			// 需要时改由控制台设备句柄承担（见 console.go）
			realStdout = resolveRenderTarget()
			realStderr = os.Stderr
			os.Stdout = stdoutW
			os.Stderr = stdoutW
			LoggerGeneral.SetOutput(stdoutW)
			intercepted = true
			go scanPipeLines(stdoutR)
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

// scanPipeLines 逐行扫描管道内容并送入原始日志缓冲。
// 同时把每一行镜像到 {程序根}/local_data/logs/console.log：进程退出后仍有据可查
// （含 Go 运行时的 panic 栈——它们同样经 stderr 进入本管道）。
func scanPipeLines(r *os.File) {
	defer r.Close()
	consoleLog := openConsoleLog()
	defer consoleLog.Close()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 落盘时剥掉 ANSI 着色序列：文件是给人读的，转义码只会干扰排查
		consoleLog.WriteLine(ansi.Strip(line))
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "stdout",
			Level:  "RAW",
			Text:   line,
		}, SourceRaw)
	}
	if err := scanner.Err(); err != nil {
		consoleLog.WriteLine("stdout 管道扫描异常: " + err.Error())
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "stdout",
			Level:  "ERROR",
			Text:   "stdout 管道扫描异常: " + err.Error(),
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
	// 绘图（sd.cpp）模块编排日志：与引擎输出合并展示于绘图引擎视图
	case module == drawModule:
		pushLog(line, SourceDraw)
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

// PushEngineLog 绘图引擎（sd.cpp）单行日志出口：由 multimodal 模块的引擎日志钩子调用。
// 同时并入原始日志与绘图引擎视图（与 PushTaskLog 同构）。
func PushEngineLog(stream, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	l := LogLine{
		Time:   time.Now(),
		Source: "engine",
		Level:  classifyEngineLevel(line),
		Module: "sd.cpp",
		Sub:    stream,
		Text:   line,
	}
	pushLog(l, SourceRaw)
	pushLog(l, SourceDraw)
}

// classifyEngineLevel 依文本推断引擎日志级别（引擎输出无级别字段）
func classifyEngineLevel(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(line, "ERROR"), strings.Contains(lower, "error"), strings.Contains(lower, "fail"):
		return "ERROR"
	case strings.Contains(line, "WARN"), strings.Contains(lower, "warn"):
		return "WARN"
	default:
		return "INFO"
	}
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
