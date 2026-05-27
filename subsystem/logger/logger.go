package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	boldRed = "\033[1;31m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

var (
	mu      sync.RWMutex
	devMode bool
	stdLog  = log.New(os.Stdout, "", 0)
)

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

func Info(module, format string, v ...interface{}) {
	mu.RLock()
	dev := devMode
	mu.RUnlock()
	if dev {
		msg := fmt.Sprintf(format, v...)
		stdLog.Printf("%s[%s]%s -> %s\n", cyan, module, reset, msg)
	}
}

func SubInfo(module, sub, format string, v ...interface{}) {
	mu.RLock()
	dev := devMode
	mu.RUnlock()
	if dev {
		msg := fmt.Sprintf(format, v...)
		stdLog.Printf("%s[%s]-[%s]%s -> %s\n", cyan, module, sub, reset, msg)
	}
}

func Warn(module, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][WARN]%s -> %s%s%s\n", yellow, module, reset, yellow, msg, reset)
}

func SubWarn(module, sub, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][WARN]%s -> %s%s%s\n", yellow, module, sub, reset, yellow, msg, reset)
}

func Error(module, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][ERROR]%s -> %s%s%s\n", red, module, reset, red, msg, reset)
}

func SubError(module, sub, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s]-[%s][ERROR]%s -> %s%s%s\n", red, module, sub, reset, red, msg, reset)
}

func Fatal(module, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("%s[%s][FATAL]%s -> %s%s%s\n", boldRed, module, reset, boldRed, msg, reset)
	os.Exit(1)
}