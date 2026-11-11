//go:build windows

package desktop

// ==== 全局变量与常量集中定义 ====
// 项目所有包级变量/常量统一在此声明，禁止与业务逻辑混在同一文件。
// 「桌面操作」族的成员各自持有自己的 AgentID / 显示名 / 提示词 / 工具白名单，
// 这里只保留族级共享的部分。

import (
	"errors"
	"time"
)

// ErrCanceled 桌面操纵任务被调用方中断（控制台手动中断 / 对话者开始应答时的自动终止）。
var ErrCanceled = errors.New("桌面任务已中断")

// AutoMaxLoops 执行循环上限：配合「连续无变化即停滞」检测，避免无限空转。
const AutoMaxLoops = 10

// autoMaxAttempts 交接记录中保留的最近尝试条数（供规划者避免重复）。
const autoMaxAttempts = 6

const (
	// DesktopCallTimeout 单个桌面原语（UIA / 键鼠 / 截图）的最长执行时间。
	DesktopCallTimeout = 10 * time.Second
	// UIADumpTimeout 读取 UIA 树的最长执行时间。
	UIADumpTimeout = 5 * time.Second
)
