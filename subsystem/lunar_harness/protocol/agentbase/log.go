package agentbase

// log.go 协议层的日志注入面。
//
// protocol 层不直接依赖 engine/console/logger：这里只暴露无依赖的回调，
// 由装配层（app/create.go）把统一日志层接进来；未注入时退回标准库日志。

import (
	"fmt"
	"log"
)

// SetLogHook 注入日志回调（level: warn / error）。传 nil 恢复标准库输出。
func SetLogHook(fn func(level, message string)) {
	logMu.Lock()
	defer logMu.Unlock()
	logHook = fn
}

// emitLog 输出一条协议层日志；未注入回调时走标准库 log。
func emitLog(level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	logMu.Lock()
	hook := logHook
	logMu.Unlock()
	if hook != nil {
		hook(level, message)
		return
	}
	log.Printf("[%s] %s", level, message)
}

// logWarn 输出警告日志。
func logWarn(format string, args ...any) {
	emitLog("warn", format, args...)
}

// logError 输出错误日志。
func logError(format string, args ...any) {
	emitLog("error", format, args...)
}
