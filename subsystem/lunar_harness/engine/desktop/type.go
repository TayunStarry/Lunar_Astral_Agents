//go:build windows

package desktop

// ==== 桌面原语类型集中定义 ====

// Window 顶层窗口的概要信息。
type Window struct {
	Title      string `json:"title"` // 窗口标题
	lowerTitle string // 小写标题缓存，供排序比较使用
	Class      string `json:"class"`   // 窗口类名
	PID        uint32 `json:"pid"`     // 进程ID
	Process    string `json:"process"` // 进程名
}
