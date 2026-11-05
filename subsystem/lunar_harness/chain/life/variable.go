package life

import (
	"math/rand"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/LifeMemorizer"
	"LunarSubsystem/LunarHarness/engine/affection"
	"LunarSubsystem/LunarHarness/engine/memory"
)

const (
	// Module 生活体系日志模块名
	Module = "Life"
	// stateFile 生活状态文件（相对 LocalDir）
	stateFile = "database/life/life_state.json"
	// memoryPlanFile 记忆整理批次计划（相对 LocalDir）
	memoryPlanFile = "database/life/memory_plan.json"
	// creativeIntentFile 生活创作的参考字符串（相对 LocalDir）
	creativeIntentFile = "database/life/creative_intent.txt"
	// memoryJournalDir 记忆整理批次意图日志目录（相对 LocalDir）
	memoryJournalDir = "database/life/memory_journal"
	// defaultIdleThreshold 默认生活冷却：600 个空闲 Tick（10 分钟）
	defaultIdleThreshold = 600
	// planWindow 未来计划摘要窗口
	planWindow = time.Hour
	// memoryReplaceSource 整理后新条目的来源标记，便于追溯与排除
	memoryReplaceSource = "consolidation"
	// memoryPlanVersion 记忆整理计划格式版本
	// 2：计划只包含待合并组；旧版（1）按时间窗口分批的计划不再续跑
	memoryPlanVersion = 2
	// memoryClusterThreshold 相似条目聚类阈值（与引擎 ClusterDefaultThreshold 对齐）
	memoryClusterThreshold float32 = 0.90
	// memoryVerifyThreshold 整理结果与来源记忆的双向相似度校验门槛
	memoryVerifyThreshold float32 = 0.80
	// memoryArchiveFile 被整理替换掉的旧条目归档日志（相对 LocalDir）
	memoryArchiveFile = "database/life/memory_archive.jsonl"
	// viewerMemorySource 观影链写入来源；生活整理只按 Source 排除，不看正文前缀
	viewerMemorySource = "bilibili_viewer"
	// pendingGroupMaxItems 单个待合并组的条目上限（与聚类上限一致，避免模型输入过大）
	pendingGroupMaxItems = memory.ClusterMaxItems
)

// 默认事件参数：权重 / 每日额度（额度单位为「次/自然日」，跨日清零）。
//
// 权重总和固定为 100，按「发生频率 × 重要性」配比：
//
//	| 事件     | 权重 | 每日额度 | 依据                                                    |
//	|----------|------|----------|---------------------------------------------------------|
//	| watch    | 18   | 6 次     | 高频休闲，一天刷几次视频                                 |
//	| daze     | 40   | 几乎不限 | 兜底事件，无模型开销；保持最高的基础占比                  |
//	| forum    | 12   | 6 次     | 中频休闲，比刷视频略低                                    |
//	| memory   | 20   | 12 次    | 只在有待合并组时工作，高权重保证待合并队列及时排空        |
//	| creative | 10   | 32 次    | 低频高价值；受创作参考字符串是否更新约束                  |
//
// memory 事件的 Weight() 在没有待合并条目时返回 0，因此日常随机挑选里
// 它只会「有待合并组」时参与，权重 20 不会挤占其它事件的常态频率。
var defaultEvents = map[string]struct {
	Weight int
	Quota  int
}{
	"watch":    {Weight: 18, Quota: 6},
	"daze":     {Weight: 40, Quota: 99999},
	"forum":    {Weight: 12, Quota: 6},
	"memory":   {Weight: 20, Quota: 12},
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

	// creativeMu 保护创作参考字符串的读写与条件重置
	creativeMu sync.Mutex

	// planDueHook 到期计划入队（由 lunar 注入，返回入队条数）
	planDueHook func() int
	// planUpcomingHook 未来窗口内计划摘要（由 lunar 注入）
	planUpcomingHook func(time.Time, time.Duration) string
	// lastUserTextHook 最新用户发言（由 lunar 注入）
	lastUserTextHook func() string
	// dialogueRoundsHook 各角色最近若干轮的成对交互（由 lunar 注入）
	dialogueRoundsHook func() []affection.Round
	// lastMoodInput 上次已用于更新心情的最新发言，避免同一发言重复嵌入
	lastMoodInput string
	// idleThreshold 可通过配置覆盖（当前先用默认值）
	idleThreshold = defaultIdleThreshold

	// 以下为外部依赖注入点：生产环境直接落到引擎/智能体，测试替换为确定性桩，
	// 避免单元测试依赖全局记忆库实例、真实嵌入服务与模型服务。

	// pendingCountHook 待处理（待合并）条目计数：为 0 时整理事件不参与随机挑选。
	pendingCountHook = memory.PendingCount
	// pendingClearHook 清除单条记忆的待处理标记（孤儿待处理条目回收使用）。
	pendingClearHook = memory.ClearPending
	// mergeHook 待合并组的模型合并改写。
	mergeHook = LifeMemorizer.Merge
	// verifyHook 整理结果与来源记忆的双向语义校验。
	verifyHook = memory.VerifyReplacementSimilarity
	// replaceHook 记忆整理的事务替换（先写新、校验通过后再删旧）。
	replaceHook = memory.ReplaceTexts
	// pickRandHook 权重挑选的随机源；生产用 math/rand.Intn，测试可替换为确定性序列。
	pickRandHook = rand.Intn
)
