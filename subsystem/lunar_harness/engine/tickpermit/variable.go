package tickpermit

// variable.go 特许运行的默认参数与进程级控制器。

import "time"

const (
	// DefaultTTL 特许运行默认有效期。
	// 必须大于 LTPX 工具自身的 30 分钟超时，给工具返回与状态复位留出余量；
	// 超过该期限仍未撤销时由调度器强制回收，避免全局状态把系统卡死。
	DefaultTTL = 35 * time.Minute
	// EndWaitTimeout 关闭特许时等待在途响应步骤结束的上限。
	// 响应步骤是普通原子步骤，通常远小于该值；超时只记错误并继续，避免反向卡死对话。
	EndWaitTimeout = 60 * time.Second
	// DefaultOwner 人格对话的特许归属名。
	DefaultOwner = "dialogue"
)

// defaultController 进程级默认控制器。
var defaultController = New()

// Default 返回进程级默认控制器。
func Default() *Controller {
	return defaultController
}
