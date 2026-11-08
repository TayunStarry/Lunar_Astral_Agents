package tickpermit

// type.go 特许运行的类型定义。
//
// 特许运行是「思考链显式让出 Tick」的唯一通道：人格对话阻塞在 LTPX 工具等待上时，
// 置位本许可，允许外部响应链在同一时刻使用模型；工具返回后撤销许可。

import (
	"sync"
	"time"
)

// Permit 一次特许运行的凭据；Release 幂等。
type Permit struct {
	controller *Controller
	generation uint64
}

// Controller 特许运行控制器。
//
// 状态只有四个：是否有效、归属、过期时间、是否有在途响应步骤。
// 所有读写都在同一把锁内完成，调度器与工具调用方共享这一个事实来源。
type Controller struct {
	mu              sync.Mutex
	active          bool
	owner           string
	generation      uint64
	expiresAt       time.Time
	responseRunning bool
	// changed 状态变化广播（关闭后替换为新 channel），用于 End 的有界等待。
	changed chan struct{}
}
