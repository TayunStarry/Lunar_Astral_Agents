package module

import "sync"

// 支持的视频格式列表
var supportedVideoFormats = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm", ".m4v"}

// TaskQueue 生成任务队列
var TaskQueue = make(chan GenerateTask, 10)

// TaskStatus 任务状态映射
var TaskStatus = make(map[string]*GenerateTask)

// TaskStatusMu 任务状态映射互斥锁
var TaskStatusMu sync.RWMutex

// WaitClients 等待任务映射
var WaitClients = make(map[string]chan *GenerateTask)

// WaitClientsMu 等待任务映射互斥锁
var WaitClientsMu sync.RWMutex

// 截图互斥锁
var ScreenshotMutex sync.RWMutex

// 最后截图时间和频率限制
var (
	LastCapture     int64                    // 最后截图时间（UnixNano）
	CaptureCooldown int64 = 50 * 1000 * 1000 // 50ms 最小截图间隔（纳秒）
)
