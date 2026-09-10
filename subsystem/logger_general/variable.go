package LoggerGeneral

import (
	"log"
	"os"
	"sync"
	"time"
)

// interval 非开发者模式下每个 module 每秒最多输出的日志条数窗口
const interval = time.Second

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
	lastLogAt  = make(map[string]time.Time)
)