package Savior

// variable.go 故障队列、升级状态与固定参数。

import (
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
	// escalateAfter 同一 agent+kind 连续出现多少次后升级为需人工介入
	escalateAfter = 3
	// historyResetWindow 超过该时长没有复发，则重新计数（按「连续」而非「累计」判定）
	historyResetWindow = 15 * time.Minute
	// maxEscalated 同时保留的升级记录上限
	maxEscalated = 16
	// salvageCooldown 放弃后对同一智能体的静默时长
	salvageCooldown = 5 * time.Minute
	// maxToolRounds 单次修复的工具循环上限
	maxToolRounds = 6
	// maxRepairActions 单次修复允许的动作次数上限（不含 finish）
	maxRepairActions = 8
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second

	// toolFailureBrief 读取结构化故障简报
	toolFailureBrief = "failure_brief"
	// toolInspectTarget 读取目标配置与运行态
	toolInspectTarget = "inspect_target"
	// toolFixTarget 执行修复动作
	toolFixTarget = "fix_target"
	// toolVerifyTarget 验证修复是否生效
	toolVerifyTarget = "verify_target"
	// toolEscalate 标记需人工介入
	toolEscalate = "escalate"
	// toolFinish 结束本次修复
	toolFinish = "finish"
	// toolReadContext 读取上下文（兜底，严格限量）
	toolReadContext = "read_context"

	// statusFixed 已修复
	statusFixed = "fixed"
	// statusNeedsHuman 需人工介入
	statusNeedsHuman = "needs_human"
	// statusUnfixable 当前手段修不了
	statusUnfixable = "unfixable"

	// recentMessageLimit 兜底读取上下文的默认消息条数
	recentMessageLimit = 6
	// maxInspectRunes 一次回显给模型的上下文/状态字符上限
	maxInspectRunes = 4000
	// maxBriefRunes 单个故障文本字段的上限
	maxBriefRunes = 512
	// maxSummaryRunes finish 结论的长度上限
	maxSummaryRunes = 400
)

var (
	// mu 保护故障队列、历史与升级记录
	mu sync.Mutex
	// pending 去重键 → 最近一条故障
	pending = map[string]*Failure{}
	// order 故障到达顺序（去重后）
	order []string
	// cooldown 智能体 → 静默截止时间
	cooldown = map[string]time.Time{}
	// history 去重键 → 连续出现次数
	history = map[string]*faultHistory{}
	// escalated 去重键 → 已升级记录
	escalated = map[string]*escalation{}
	// seq 故障编号
	seq int64
)
