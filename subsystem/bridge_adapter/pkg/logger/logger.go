package logger

import (
	centrallog "logger"
	"os"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var logLevel = INFO

func init() {
	centrallog.SetDevMode(true)
}

func SetLogLevel(level LogLevel) {
	logLevel = level
}

func SetLogWriter(writer *os.File) {
	centrallog.SetOutput(writer)
}

func Debug(format string, v ...interface{}) {
	if logLevel <= DEBUG {
		centrallog.Info("BridgeAdapter", format, v...)
	}
}

func Info(format string, v ...interface{}) {
	if logLevel <= INFO {
		centrallog.Info("BridgeAdapter", format, v...)
	}
}

func Warn(format string, v ...interface{}) {
	if logLevel <= WARN {
		centrallog.Error("BridgeAdapter", format, v...)
	}
}

func Error(format string, v ...interface{}) {
	if logLevel <= ERROR {
		centrallog.Error("BridgeAdapter", format, v...)
	}
}

func Fatal(format string, v ...interface{}) {
	centrallog.Fatal("BridgeAdapter", format, v...)
}