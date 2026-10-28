package logger

import (
	"log"
	"os"
	"sync"
)

// notSub 无子标题的日志函数（如 Info/Warn/Error/Fatal）使用的占位子标题
const notSub = "not"

// 日志级别常量
const (
	levelInfo  = "INFO"
	levelWarn  = "WARN"
	levelError = "ERROR"
	levelFatal = "FATAL"
)

// 控制台 ANSI 色彩码
const (
	reset   = "\033[0m"
	red     = "\033[31m"
	boldRed = "\033[1;31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
)

var (
	mu      sync.RWMutex // 保护 devMode / stdLog
	devMode bool
	stdLog  = log.New(os.Stdout, "", 0)

	hookMu      sync.Mutex                           // 保护 logHook 的并发读写
	logHookFunc func(level, module, sub, msg string) // 外部流式日志钩子（TUI 管线注册；nil 表示未启用）

	alertMu       sync.Mutex                           // 保护 alertHook 的并发读写
	alertHookFunc func(level, module, sub, msg string) // 告警钩子（开发者模式下警告及以上事件；nil 表示未启用）
)
