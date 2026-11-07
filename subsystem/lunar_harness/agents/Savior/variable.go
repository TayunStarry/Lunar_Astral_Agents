package Savior

// variable.go 故障队列、升级状态与固定参数。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// roleLabel 控制台显示名（同时作为日志模块名，归入「系统日志 · 后勤」）
	roleLabel = agentinfo.NameSavior
	// maxPending 同时保留的故障上限
	maxPending = 8
	// maxAttempts 同一个故障最多诊断尝试次数（模型/工具本身失败时重试）
	maxAttempts = 2
	// escalateAfter 同一 agent+kind 连续出现多少次后标记为需人工跟进
	escalateAfter = 3
	// historyResetWindow 超过该时长没有复发，则重新计数（按「连续」而非「累计」判定）
	historyResetWindow = 15 * time.Minute
	// maxEscalated 同时保留的升级记录上限
	maxEscalated = 16
	// maxAdvices 同时保留的维修建议上限
	maxAdvices = 16
	// salvageCooldown 放弃后对同一智能体的静默时长
	salvageCooldown = 5 * time.Minute
	// maxToolRounds 单次诊断的工具循环上限
	maxToolRounds = 6
	// maxDiagnoseActions 单次诊断允许的只读动作次数上限（不含 finish）
	maxDiagnoseActions = 8
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second

	// toolFailureBrief 读取结构化故障简报
	toolFailureBrief = "failure_brief"
	// toolInspectTarget 读取目标配置与运行态
	toolInspectTarget = "inspect_target"
	// toolReadContext 读取上下文（兜底，严格限量）
	toolReadContext = "read_context"
	// toolFinish 提交结构化维修建议并结束本次诊断
	toolFinish = "finish"

	// confidenceHigh 置信度：依据充分
	confidenceHigh = "high"
	// confidenceMedium 置信度：依据一般
	confidenceMedium = "medium"
	// confidenceLow 置信度：依据不足
	confidenceLow = "low"

	// recentMessageLimit 兜底读取上下文的默认消息条数
	recentMessageLimit = 6
	// maxInspectRunes 一次回显给模型的上下文/状态字符上限
	maxInspectRunes = 4000
	// maxBriefRunes 单个故障文本字段的上限
	maxBriefRunes = 512
	// maxAdviceRunes 单个建议字段的长度上限
	maxAdviceRunes = 600
	// maxAdviceSteps 一条建议最多保留的步骤数
	maxAdviceSteps = 6
)

var (
	// mu 保护故障队列、历史、建议与升级记录
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
	// advices 去重键 → 最近一条维修建议
	advices = map[string]*Advice{}
	// seq 故障编号
	seq int64
)
