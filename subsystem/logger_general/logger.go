package LoggerGeneral

import (
	"fmt"
	"log"
	"os"
)

// SetDevMode 设置开发模式开关
// v: 是否启用开发模式（开发者模式下所有日志输出到控制台且不受去重限制）
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

// shouldPrint 判断是否允许输出。
// 开发者模式下所有日志无限制；非开发者模式下，若本次调用的 module 和 sub
// 与上一次调用完全相同，则不输出。
func shouldPrint(module, sub string) bool {
	mu.Lock()
	defer mu.Unlock()
	if devMode {
		return true
	}
	if hasLast && lastModule == module && lastSub == sub {
		return false
	}
	hasLast = true
	lastModule = module
	lastSub = sub
	return true
}

func Info(module, format string, v ...any) {
	if !shouldPrint(module, notSub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]%s -> %s\n", cyan, module, reset, msg)
}

func SubInfo(module, sub, format string, v ...any) {
	if !shouldPrint(module, sub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s]%s -> %s\n", cyan, module, sub, reset, msg)
}

func Warn(module, format string, v ...any) {
	if !shouldPrint(module, notSub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][WARN]%s -> %s%s%s\n", yellow, module, reset, yellow, msg, reset)
}

func SubWarn(module, sub, format string, v ...any) {
	if !shouldPrint(module, sub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][WARN]%s -> %s%s%s\n", yellow, module, sub, reset, yellow, msg, reset)
}

func Error(module, format string, v ...any) {
	if !shouldPrint(module, notSub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][ERROR]%s -> %s%s%s\n", red, module, reset, red, msg, reset)
}

func SubError(module, sub, format string, v ...any) {
	if !shouldPrint(module, sub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][ERROR]%s -> %s%s%s\n", red, module, sub, reset, red, msg, reset)
}

func Fatal(module, format string, v ...any) {
	if !shouldPrint(module, notSub) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][FATAL]%s -> %s%s%s\n", boldRed, module, reset, boldRed, msg, reset)
	os.Exit(1)
}
