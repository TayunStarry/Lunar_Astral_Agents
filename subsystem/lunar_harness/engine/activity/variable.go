package activity

import "sync"

const (
	// stateFile 活动状态文件（相对 LocalDir），与心情状态同目录。
	stateFile = "database/life/activity_state.json"
	// fieldPrefix 运行时消息里的固定字段前缀（中文全角冒号）。
	fieldPrefix = "我刚刚在干的事情："
	// defaultText 未设置时的默认描述，保证字段永远不为空。
	defaultText = "在安静待着，随时准备回应你"
	// stateVersion 当前状态文件版本。
	stateVersion = 1
)

var (
	mu     sync.RWMutex
	state  State
	loaded bool
)
