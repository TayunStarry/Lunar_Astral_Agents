package LoggerGeneral

import (
	"fmt"
	"log"
	"os"
	"time"
)

// SetDevMode 设置开发模式开关
// v: 是否启用开发模式（开发者模式下所有日志输出到控制台且无频率限制）
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

// rateLimit 判断是否允许输出。
// 开发者模式下所有日志无频率限制；非开发者模式下每个 module 一秒最多输出一条。
func rateLimit(module string) bool {
	mu.Lock()
	defer mu.Unlock()
	if devMode {
		return true
	}
	now := time.Now()
	if last, ok := lastLogAt[module]; ok && now.Sub(last) < interval {
		return false
	}
	lastLogAt[module] = now
	return true
}

func Info(module, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]%s -> %s\n", cyan, module, reset, msg)
}

func SubInfo(module, sub, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s]%s -> %s\n", cyan, module, sub, reset, msg)
}

func Warn(module, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][WARN]%s -> %s%s%s\n", yellow, module, reset, yellow, msg, reset)
}

func SubWarn(module, sub, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][WARN]%s -> %s%s%s\n", yellow, module, sub, reset, yellow, msg, reset)
}

func Error(module, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][ERROR]%s -> %s%s%s\n", red, module, reset, red, msg, reset)
}

func SubError(module, sub, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][ERROR]%s -> %s%s%s\n", red, module, sub, reset, red, msg, reset)
}

func Fatal(module, format string, v ...any) {
	if !rateLimit(module) {
		return
	}
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][FATAL]%s -> %s%s%s\n", boldRed, module, reset, boldRed, msg, reset)
	os.Exit(1)
}
