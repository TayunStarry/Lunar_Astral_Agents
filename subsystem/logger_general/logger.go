package LoggerGeneral

import (
	"fmt"
	"html"
	"log"
	"os"
)

// SetDevMode 设置开发模式开关
// v: 是否启用开发模式（开发者模式下打印全部日志，非开发者模式仅打印警告及以上级别）
func SetDevMode(v bool) {
	mu.Lock()
	defer mu.Unlock()
	devMode = v
}

func SetOutput(w *os.File) {
	mu.Lock()
	defer mu.Unlock()
	stdLog = log.New(w, "", 0)
}

// shouldPrint 判断是否允许输出并记录。
// 开发者模式下打印全部日志；非开发者模式下仅打印（记录）警告、错误、致命级别日志。
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

// emit 统一输出：级别过滤 → 控制台打印（ANSI 色彩）→ 环形缓冲记录（MD 风格文本）
func emit(level, module, sub, format string, v ...any) {
	if !shouldPrint(level) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	tag := module
	if sub != "" && sub != notSub {
		tag = module + "-" + sub
	}
	var ansiColor, mdColor string
	var mdBold bool
	var levelSuffix string
	switch level {
	case levelInfo:
		ansiColor, mdColor, mdBold = cyan, mdCyan, false
	case levelWarn:
		ansiColor, mdColor, mdBold, levelSuffix = yellow, mdYellow, true, "[WARN]"
	case levelError:
		ansiColor, mdColor, mdBold, levelSuffix = red, mdRed, true, "[ERROR]"
	case levelFatal:
		ansiColor, mdColor, mdBold, levelSuffix = boldRed, mdRed, true, "[FATAL]"
	}

	// 控制台输出（ANSI 色彩，与历史格式保持一致）
	if level == levelInfo {
		stdLog.Printf("%s[%s]%s -> %s\n", ansiColor, tag, reset, msg)
	} else {
		stdLog.Printf("%s[%s%s]%s -> %s%s%s\n", ansiColor, tag, levelSuffix, reset, ansiColor, msg, reset)
	}

	// 环形缓冲记录（MD 风格：HTML 色彩标注 + 加粗，消息体已做 HTML 转义）
	mdTag := fmt.Sprintf("<span style=\"color:%s\"><b>[%s%s]</b></span>", mdColor, tag, levelSuffix)
	mdBody := html.EscapeString(msg)
	if mdBold {
		mdBody = fmt.Sprintf("<b><span style=\"color:%s\">%s</span></b>", mdColor, mdBody)
	}
	recordLog(level, module, sub, fmt.Sprintf("%s → %s", mdTag, mdBody))
}

// recordLog 将一条日志写入环形缓冲（仅保留最新 logRingSize 条）
func recordLog(level, module, sub, text string) {
	logMu.Lock()
	defer logMu.Unlock()
	logRing[logCount%logRingSize] = LogRecord{
		Level:  level,
		Module: module,
		Sub:    sub,
		Text:   text,
	}
	logCount++
}

// GetRecentLogs 返回最新的最多 n 条日志记录（按时间先后排序）。
// n <= 0 或超过容量时返回当前全部有效记录（最多 1000 条）。
func GetRecentLogs(n int) []LogRecord {
	logMu.Lock()
	defer logMu.Unlock()
	size := logCount
	if size > logRingSize {
		size = logRingSize
	}
	if n > 0 && n < size {
		size = n
	}
	if size <= 0 {
		return []LogRecord{}
	}
	result := make([]LogRecord, 0, size)
	start := logCount - size
	for i := 0; i < size; i++ {
		result = append(result, logRing[(start+i)%logRingSize])
	}
	return result
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
