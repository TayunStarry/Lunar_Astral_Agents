package LoggerGeneral

import (
	"log"
	"os"
	"sync"
)

// notSub 无子标题的日志函数（如 Info/Warn/Error/Fatal）使用的占位子标题
const notSub = "not"

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	boldRed = "\033[1;31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
)

var (
	mu         sync.RWMutex
	devMode    bool
	stdLog     = log.New(os.Stdout, "", 0)
	hasLast    bool
	lastModule string
	lastSub    string
)
