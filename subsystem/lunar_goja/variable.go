package LunarGoja

import (
	"sync"
	"time"

	"github.com/dop251/goja_nodejs/eventloop"
)

// ==== 通用 goja 运行时全局变量 ====

// 全局变量，存储 goja 事件循环实例与生命周期锁
var (
	// runtimeMutex 保护运行时生命周期（初始化/关闭/查询）
	runtimeMutex sync.Mutex
	// loop 全局 goja 事件循环实例（nil 表示未初始化）
	loop *eventloop.EventLoop
)

// DefaultFetchTimeout fetch 未显式指定 timeout 时的默认请求超时
// 默认 300 秒，与 lunar_astral 的 sync-fetch-timeout 默认值保持一致，可按需调整
var DefaultFetchTimeout = 300 * time.Second
