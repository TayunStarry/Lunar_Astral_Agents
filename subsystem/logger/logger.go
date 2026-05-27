package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
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
		stdLog.Printf("[%s] -> %s\n", module, msg)
	}
}

func SubInfo(module, sub, format string, v ...interface{}) {
	mu.RLock()
	dev := devMode
	mu.RUnlock()
	if dev {
		msg := fmt.Sprintf(format, v...)
		stdLog.Printf("[%s]-[%s] -> %s\n", module, sub, msg)
	}
}

func Error(module, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("[%s][ERROR] -> %s\n", module, msg)
}

func SubError(module, sub, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("[%s]-[%s][ERROR] -> %s\n", module, sub, msg)
}

func Fatal(module, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	stdLog.Printf("[%s][ERROR] -> %s\n", module, msg)
	os.Exit(1)
}