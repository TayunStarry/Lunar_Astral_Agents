package execute

import "sync"

// 最大队列长度
var maxQueueLength = 3
// 当前正在处理的请求数
var currentProcessing int
// 请求队列
var requestQueue []chan struct{}
// 队列锁
var queueMutex sync.Mutex
