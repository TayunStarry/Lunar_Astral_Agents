package life

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/LifeMemorizer"
	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/wishchain"
)

const (
	// Module 生活体系日志模块名
	Module = "Life"
	// stateFile 生活状态文件（相对 LocalDir）
	stateFile = "database/life/life_state.json"
	// memoryPlanFile 记忆整理批次计划（相对 LocalDir）
	memoryPlanFile = "database/life/memory_plan.json"
	// memoryJournalDir 记忆整理批次意图日志目录（相对 LocalDir）
	memoryJournalDir = "database/life/memory_journal"
	// defaultIdleThreshold 默认生活冷却：600 个空闲 Tick（10 分钟）
	defaultIdleThreshold = 600
	// wishWindow 未来愿望摘要窗口
	wishWindow = time.Hour
	// memoryReplaceSource 整理后新条目的来源标记，便于追溯与排除
	memoryReplaceSource = "consolidation"
	// memoryPlanVersion 记忆整理计划格式版本
	// 2：计划只包含待合并组；旧版（1）按时间窗口分批的计划不再续跑
	memoryPlanVersion = 2
	// memoryClusterThreshold 相似条目聚类阈值（与引擎 ClusterDefaultThreshold 对齐）
	memoryClusterThreshold float32 = 0.90
	// memoryVerifyThreshold 整理结果与来源记忆的双向相似度校验门槛
	memoryVerifyThreshold float32 = 0.80
	// pendingMaxAttempts 同一批待合并条目连续整理失败的上限；达到后解除待处理标记
	pendingMaxAttempts = 3
	// pendingMaxAge 待处理条目最长隐身时长；超时后即使仍失败也放行可见
	pendingMaxAge = 72 * time.Hour
	// memoryArchiveFile 被整理替换掉的旧条目归档日志（相对 LocalDir）
	memoryArchiveFile = "database/life/memory_archive.jsonl"
	// viewerMemorySource 观影链写入来源；生活整理只按 Source 排除，不看正文前缀
	viewerMemorySource = "bilibili_viewer"
	// pendingGroupMaxItems 单个待合并组的条目上限（与聚类上限一致，避免模型输入过大）
	pendingGroupMaxItems = memory.ClusterMaxItems
	// forumPostCooldown 自主发帖冷却：回复不限次，发新话题至少隔这么久，避免每轮都刷帖
	forumPostCooldown = 6 * time.Hour
	// activityTextLimit 状态字段文案上限（按句读截断，避免长记录把状态撑爆）
	activityTextLimit = 60
	// activityReasonLimit 失败 / 早退原因在状态字段里的长度上限
	activityReasonLimit = 40
)

// 默认事件参数：权重 / 每日额度（额度单位为「次/自然日」，跨日清零）。
//
// 权重总和固定为 100，按「发生频率 × 重要性」配比：
//
//	| 事件     | 权重 | 每日额度 | 依据                                                      |
//	|----------|------|----------|-----------------------------------------------------------|
//	| daze     | 40   | 几乎不限 | 兜底事件，无模型开销；仍是最高占比                          |
//	| watch    | 26   | 12 次    | 正式分支；有已下载片段才有意义，占用最高的资源               |
//	| forum    | 20   | 6 次     | 模型驱动休闲；发帖另有冷却，权重不宜过高                     |
//	| creative | 14   | 32 次    | 低频高价值；超过额度后自然退出挑选                           |
//
// 记忆整理不再是权重事件：它和更新心情、更新对他人态度一样，是每轮固定步骤，
// 因此不占权重、没有额度，也不可能被事件挑选命中。移除 memory 的 16 点权重后，
// 其余事件按原比例重新归一（34→40 / 22→26 / 16→20 / 12→14），总和仍为 100。
var defaultEvents = map[string]struct {
	Weight int
	Quota  int
}{
	"daze":     {Weight: 40, Quota: 99999},
	"watch":    {Weight: 26, Quota: 12},
	"forum":    {Weight: 20, Quota: 6},
	"creative": {Weight: 14, Quota: 32},
	// 工具/命令触发的子链路不参与权重抽签，只接受强制事件。
	"storyboard": {Weight: 0, Quota: 0},
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

	// wishDueHook 到期愿望入队（由 lunar 注入，返回入队条数）
	wishDueHook func() int
	// wishUpcomingHook 未来窗口内愿望摘要（由 lunar 注入）
	wishUpcomingHook func(time.Time, time.Duration) string
	// dialogueRoundsHook 各角色最近若干轮的成对交互（由 lunar 注入）
	dialogueRoundsHook func() []wishchain.Round
	// idleThreshold 可通过配置覆盖（当前先用默认值）
	idleThreshold = defaultIdleThreshold

	// 以下为外部依赖注入点：生产环境直接落到引擎/智能体，测试替换为确定性桩，
	// 避免单元测试依赖全局记忆库实例、真实嵌入服务与模型服务。

	// pendingCountHook 待处理（待合并）条目计数：为 0 时固定步骤三直接空操作。
	pendingCountHook = memory.PendingCount
	// pendingClearHook 清除单条记忆的待处理标记（整理终止与孤儿回收使用）。
	pendingClearHook = memory.ClearPending
	// pendingFailureHook 记录一次整理失败：累计尝试次数，达到上限或超时后解除待处理标记。
	pendingFailureHook = memory.RecordPendingFailure
	// mergeHook 待合并组的模型合并改写。
	mergeHook = LifeMemorizer.Merge
	// verifyHook 整理结果与来源记忆的双向语义校验。
	verifyHook = memory.VerifyReplacementSimilarity
	// replaceHook 记忆整理的事务替换（先写新、校验通过后再删旧）。
	replaceHook = memory.ReplaceTexts
	// pickRandHook 权重挑选的随机源；生产用 math/rand.Intn，测试可替换为确定性序列。
	pickRandHook = rand.Intn

	// 以下为每轮固定步骤的注入点，测试可替换为记录调用顺序的桩。
	// 顺序在 steps.go 的 runFixedSteps 中显式书写，不依赖任何表遍历。

	// wishStepHook 固定步骤一：结算愿望并联动心情。
	wishStepHook = settleWishesStep
	// moodStepHook 固定步骤二：更新心情。
	moodStepHook = updateMoodStep
	// affectionStepHook 固定步骤三：更新对他人态度。
	affectionStepHook = updateAffectionStep
	// memoryPendingStepHook 固定步骤四：合并记忆待处理条目。
	memoryPendingStepHook = mergePendingMemories
	// memoryMergeRunHook 固定步骤四的实际整理执行器；测试可替换以统计调用次数。
	memoryMergeRunHook = runMemoryBatches

	// 以下为回合第 2~4 步「机会性维护」的注入点。
	// 三步按固定顺序执行，任一步失败或条件不满足都只记录、不中断后续步骤。

	// ensureWatchlistHook 第 2 步：待看列表为空时补齐候选、审核、入库与切片下载。
	ensureWatchlistHook func(context.Context) (viewer.WatchlistResult, error) = viewer.EnsureWatchlist
	// biliAutoRespondHook 第 3 步：回应 B 站 @ 与自己的视频/动态评论区。
	biliAutoRespondHook func(context.Context) (viewer.AutoRespondResult, error) = viewer.AutoRespondOnBilibili
	// forumAutoRespondHook 第 4 步：回应互助论坛的未读消息。
	forumAutoRespondHook func(context.Context) (MutualForum.Summary, error) = MutualForum.AutoRespondUnread
	// pauseHook 协作式让出判定：复用观影链的 SetPauseHook / ErrPaused 机制，不引入抢占式中断。
	pauseHook func() bool = viewer.PauseRequested

	// watchStepHook 看视频分支：生活链权重分支与工具触发共用同一实现。
	watchStepHook func(context.Context, string) (viewer.WatchOutcome, error) = viewer.WatchNextSegment
	// setActivityHook 写入「我刚刚在干的事情」状态字段。
	setActivityHook func(string) = activity.Set
)
