package LoggerGeneral

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

// Markdown 渲染色彩（对应 ANSI 色彩码，前端按文本内嵌 HTML 渲染色彩标注）
const (
	mdCyan   = "#56b6c2"
	mdYellow = "#d19a66"
	mdRed    = "#e06c75"
)

// logRingSize 日志环形缓冲容量：最多记录最新的 1000 条日志
const logRingSize = 1000

var (
	mu      sync.RWMutex // 保护 devMode / stdLog
	devMode bool
	stdLog  = log.New(os.Stdout, "", 0)

	logMu    sync.Mutex // 保护日志环形缓冲
	logRing  [logRingSize]LogRecord
	logCount int // 已写入日志总数（同时作为环形缓冲写指针）
)
