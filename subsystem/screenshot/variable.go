package screenshot

import (
	"sync"
	"time"
)

// 截图互斥锁
var screenshotMutex sync.RWMutex

// 最后截图时间和频率限制
var (
	lastCapture     time.Time               // 最后截图时间
	captureCooldown = 50 * time.Millisecond // 最小截图间隔
)
