package logger

import (
	"log"
	"os"
	"sync"
)

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	boldRed = "\033[1;31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
)

var (
	mu      sync.RWMutex
	devMode bool
	stdLog  = log.New(os.Stdout, "", 0)
)
