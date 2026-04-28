package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	logLevel  = INFO
	logWriter = os.Stdout
	logger    = log.New(logWriter, "", 0)
)

func SetLogLevel(level LogLevel) {
	logLevel = level
}

func SetLogWriter(writer *os.File) {
	logWriter = writer
	logger = log.New(logWriter, "", 0)
}

func getTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

func getLevelPrefix(level LogLevel) string {
	switch level {
	case DEBUG:
		return "[DEBUG]"
	case INFO:
		return "[INFO ]"
	case WARN:
		return "[WARN ]"
	case ERROR:
		return "[ERROR]"
	default:
		return "[INFO ]"
	}
}

func Debug(format string, v ...interface{}) {
	if logLevel <= DEBUG {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s %s %s\n", getTimestamp(), getLevelPrefix(DEBUG), msg)
	}
}

func Info(format string, v ...interface{}) {
	if logLevel <= INFO {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s %s %s\n", getTimestamp(), getLevelPrefix(INFO), msg)
	}
}

func Warn(format string, v ...interface{}) {
	if logLevel <= WARN {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s %s %s\n", getTimestamp(), getLevelPrefix(WARN), msg)
	}
}

func Error(format string, v ...interface{}) {
	if logLevel <= ERROR {
		msg := fmt.Sprintf(format, v...)
		logger.Printf("%s %s %s\n", getTimestamp(), getLevelPrefix(ERROR), msg)
	}
}

func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	logger.Printf("%s %s %s\n", getTimestamp(), getLevelPrefix(ERROR), msg)
	os.Exit(1)
}
