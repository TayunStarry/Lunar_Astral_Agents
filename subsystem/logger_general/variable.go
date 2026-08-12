package LoggerGeneral

import (
	"log"
	"os"
	"sync"
	"time"
)

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	boldRed = "\033[1;31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
)

var (
	mu        sync.RWMutex
	devMode   bool
	stdLog    = log.New(os.Stdout, "", 0)
	startTime = time.Now()
	logFile   *os.File
)