package Savior

// variable.go 故障队列与固定参数。

import (
	"net/http"
	"sync"
	"time"
)

const (
	// roleLabel 控制台显示名（同时作为日志模块名，归入「系统日志 · 后勤」）
	roleLabel = "智能修复"
	// maxPending 同时保留的故障上限
	maxPending = 8
	// maxAttempts 同一个故障最多修复尝试次数（避免无限修）
	maxAttempts = 2
	// salvageCooldown 放弃后对同一智能体的静默时长
	salvageCooldown = 5 * time.Minute
	// maxToolRounds 单次修复的工具循环上限
	maxToolRounds = 6
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second

	// toolReadContext 读取智能体上下文
	toolReadContext = "read_context"
	// toolRepairContext 修改智能体上下文
	toolRepairContext = "repair_context"
	// toolFinish 结束本次修复
	toolFinish = "finish"
	// recentMessageLimit 默认只看最近多少条消息
	recentMessageLimit = 8
	// maxInspectRunes 一次回显给模型的上下文字符上限
	maxInspectRunes = 12000
)

var (
	// mu 保护故障队列
	mu sync.Mutex
	// pending 智能体 → 最近一次故障
	pending = map[string]*Failure{}
	// order 故障到达顺序（去重后）
	order []string
	// cooldown 智能体 → 静默截止时间
	cooldown = map[string]time.Time{}
	// seq 故障编号
	seq int64
	// httpClient 非流式模型调用客户端
	httpClient = &http.Client{Timeout: modelTimeout}
)
