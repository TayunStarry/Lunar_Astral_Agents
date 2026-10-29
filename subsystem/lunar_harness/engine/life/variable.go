package life

import (
	"sync"
	"time"
)

const (
	// Module 生活体系日志模块名
	Module = "Life"
	// stateFile 生活状态文件（相对 LocalDir）
	stateFile = "database/life_chain/life_state.json"
	// memoryPlanFile 记忆整理批次计划（相对 LocalDir）
	memoryPlanFile = "database/life_chain/memory_plan.json"
	// memoryJournalDir 记忆整理批次意图日志目录（相对 LocalDir）
	memoryJournalDir = "database/life_chain/memory_journal"
	// defaultIdleThreshold 默认生活冷却：600 个空闲 Tick（10 分钟）
	defaultIdleThreshold = 600
	// planWindow 未来计划摘要窗口
	planWindow = time.Hour
	// batchMaxRunes 记忆整理单批最大字符数
	batchMaxRunes = 10000
	// memoryMaxTextRunes 单条整理后记忆的最大字符数
	memoryMaxTextRunes = 500
	// modelRetry 记忆整理模型输出解析失败后的额外重试次数
	modelRetry = 1
)

// 默认事件参数：权重 / 每日额度。
var defaultEvents = map[string]struct {
	Weight int
	Quota  int
}{
	"watch":    {Weight: 14, Quota: 6},
	"daze":     {Weight: 50, Quota: 99999},
	"forum":    {Weight: 18, Quota: 6},
	"memory":   {Weight: 4, Quota: 2},
	"creative": {Weight: 10, Quota: 32},
}

// eventParams 单个事件的权重与额度。
type eventParams struct {
	Weight int
	Quota  int
}

func paramsOf(id string) eventParams {
	if params, ok := defaultEvents[id]; ok {
		return eventParams{Weight: params.Weight, Quota: params.Quota}
	}
	return eventParams{Weight: 1, Quota: 1}
}

var (
	mu       sync.Mutex
	state    = State{Version: 1, Used: map[string]int{}}
	registry []Event

	// planDueHook 到期计划入队（由 lunar 注入，返回入队条数）
	planDueHook func() int
	// planUpcomingHook 未来窗口内计划摘要（由 lunar 注入）
	planUpcomingHook func(time.Time, time.Duration) string
	// lastUserTextHook 最新用户发言（由 lunar 注入）
	lastUserTextHook func() string
	// lastUserNameHook 最新用户发言的发送者名（由 lunar 注入）
	lastUserNameHook func() string
	// lastReplyTextHook 上一轮月华回应文本（由 lunar 注入）
	lastReplyTextHook func() string
	// lastMoodInput 上次已用于更新心情的最新发言，避免同一发言重复嵌入
	lastMoodInput string
	// lastEmotionInput 上次已用于更新情感认知的「用户名+发言+回应」组合
	lastEmotionInput string
	// idleThreshold 可通过配置覆盖（当前先用默认值）
	idleThreshold = defaultIdleThreshold
)
