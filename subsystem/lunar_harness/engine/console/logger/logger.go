package logger

import (
	"fmt"
	"io"
	"log"
	"os"

	"LunarSubsystem/LunarHarness/engine/sound"
)

// SetDevMode 设置开发模式开关
// v: 是否启用开发模式（开发者模式下打印全部日志，非开发者模式仅打印警告及以上级别）
func SetDevMode(v bool) {
	mu.Lock()
	defer mu.Unlock()
	devMode = v
}

// SetOutput 设置底层输出目标（默认真实 stdout）。
// TUI 管线接管时传入控制台日志镜像写入器：logger 的打印不再进入 stdout 管道，
// 「原始日志」视图由日志钩子单一供源，避免同一行经两条路径重复出现。
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	stdLog = log.New(w, "", 0)
}

// SetLogHook 注册外部流式日志钩子（TUI 统一日志管线使用）。
// 钩子在级别过滤与底层打印**之前**同步回调，接收全量日志的纯文本消息
// （未经 ANSI 色彩与 Markdown 转义）；视图缓冲与环形截断由管线侧负责。
// 传 nil 注销钩子。
func SetLogHook(hook func(level, module, sub, msg string)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	logHookFunc = hook
}

// shouldPrint 判断是否允许输出到底层目标。
// 底层目标在非 TUI 模式下是真实控制台，TUI 模式下是控制台日志镜像（console.log）。
// 开发者模式下全部输出；非开发者模式下仅输出警告、错误、致命级别。
// TUI 各视图缓冲不受此过滤——它们经日志钩子接收全量。
func shouldPrint(level string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if devMode {
		return true
	}
	switch level {
	case levelWarn, levelError, levelFatal:
		return true
	}
	return false
}

// emit 统一输出：消息格式化 → 外部流式钩子（全量） → 级别过滤 → 控制台打印（ANSI 色彩）
func emit(level, module, sub, format string, v ...any) {
	msg := fmt.Sprintf(format, v...)

	// 外部流式钩子在级别过滤前调用：无论 devMode 与级别，TUI 管线都接收全量日志
	hookMu.Lock()
	hook := logHookFunc
	hookMu.Unlock()
	if hook != nil {
		hook(level, module, sub, msg)
	}

	// 警告及以上统一播报失败音效：异步且带节流，不会阻塞日志与刷屏；
	// 致命错误同步播完再退出，避免声音还没响进程就结束了。
	switch level {
	case levelWarn, levelError:
		sound.Failure()
	case levelFatal:
		sound.FailureBlocking()
	}

	if !shouldPrint(level) {
		return
	}
	tag := module
	if sub != "" && sub != notSub {
		tag = module + "-" + sub
	}
	var ansiColor, levelSuffix string
	switch level {
	case levelInfo:
		ansiColor = cyan
	case levelWarn:
		ansiColor, levelSuffix = yellow, "[WARN]"
	case levelError:
		ansiColor, levelSuffix = red, "[ERROR]"
	case levelFatal:
		ansiColor, levelSuffix = boldRed, "[FATAL]"
	}

	// 控制台输出（ANSI 色彩，与历史格式保持一致）
	if level == levelInfo {
		stdLog.Printf("%s[%s]%s -> %s\n", ansiColor, tag, reset, msg)
	} else {
		stdLog.Printf("%s[%s%s]%s -> %s%s%s\n", ansiColor, tag, levelSuffix, reset, ansiColor, msg, reset)
	}
}

func Info(module, format string, v ...any) {
	emit(levelInfo, module, notSub, format, v...)
}

func SubInfo(module, sub, format string, v ...any) {
	emit(levelInfo, module, sub, format, v...)
}

func Warn(module, format string, v ...any) {
	emit(levelWarn, module, notSub, format, v...)
}

func SubWarn(module, sub, format string, v ...any) {
	emit(levelWarn, module, sub, format, v...)
}

func Error(module, format string, v ...any) {
	emit(levelError, module, notSub, format, v...)
}

func SubError(module, sub, format string, v ...any) {
	emit(levelError, module, sub, format, v...)
}

func Fatal(module, format string, v ...any) {
	emit(levelFatal, module, notSub, format, v...)
	os.Exit(1)
}
