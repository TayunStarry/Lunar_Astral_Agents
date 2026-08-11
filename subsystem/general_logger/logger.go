package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// stripWriter 包装 io.Writer，写入前自动移除 ANSI 转义序列（颜色标记）
type stripWriter struct {
	w io.Writer
}

func (s *stripWriter) Write(p []byte) (n int, err error) {
	cleaned := make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		// 检测 ANSI 转义序列起始符 \033[
		if p[i] == 0x1b && i+1 < len(p) && p[i+1] == '[' {
			// 查找终止符 'm'
			j := i + 2
			for j < len(p) && p[j] != 'm' {
				j++
			}
			if j < len(p) {
				i = j // 跳过整个转义序列
				continue
			}
		}
		cleaned = append(cleaned, p[i])
	}
	_, err = s.w.Write(cleaned)
	return len(p), err // 返回原始长度，兼容 log.Logger 内部计数
}

// SetDevMode 设置开发模式并指定日志文件保存目录
// v: 是否启用开发模式（日志输出到控制台）
// logDir: 日志文件保存目录（为空则保存到当前目录）
func SetDevMode(v bool, logDir string) {
	mu.Lock()
	defer mu.Unlock()

	devMode = v

	// 关闭之前的日志文件
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	if v {
		// 生成日志文件名：程序启动时间.log
		filename := startTime.Format("2006-01-02_15-04-05") + ".log"

		// 未指定目录则使用当前目录
		if logDir == "" {
			logDir = "."
		}

		// 创建目录（如已存在则跳过）
		if err := os.MkdirAll(logDir, 0755); err != nil {
			stdLog.Printf("%s[logger][WARN]%s -> 创建日志目录失败: %v\n", yellow, reset, err)
			// 目录创建失败，仅输出到控制台
			return
		}

		logPath := filepath.Join(logDir, filename)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			stdLog.Printf("%s[logger][WARN]%s -> 创建日志文件失败: %v\n", yellow, reset, err)
			return
		}

		logFile = f
		// 同时输出到控制台（保留颜色）和日志文件（移除颜色标记）
		stdLog = log.New(io.MultiWriter(os.Stdout, &stripWriter{w: f}), "", 0)
	} else {
		// 关闭开发模式，恢复仅控制台输出
		stdLog = log.New(os.Stdout, "", 0)
	}
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
