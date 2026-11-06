package creation

// plan_var.go 自主创作事项的常量、情绪极性分组与可替换钩子。

import (
	"math/rand"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/mood"
)

const (
	// planStateFile 创作事项状态文件（相对 LocalDir），与心情 / 活动状态同目录
	planStateFile = "database/life/creation_plan.json"
	// planStateVersion 状态文件版本
	planStateVersion = 1
	// recentWindow 事项类型排除窗口：最近 N 次选中的类型不再立刻重抽
	recentWindow = 2
	// maxParked 暂存的未完成事项上限，超出时丢弃最早的一条
	maxParked = 8
)

// 情绪极性分组：与 engine/mood 的基础情绪词表一一对应。
var (
	// positiveMoodLabels 正向情绪
	positiveMoodLabels = []string{"开心", "满足", "元气", "好奇", "平静"}
	// negativeMoodLabels 负向情绪
	negativeMoodLabels = []string{"愤怒", "伤心", "无聊", "厌倦", "担忧", "恐慌"}
)

var (
	// planMu 串行化事项状态文件的读改写，避免并发选到不同事项
	planMu sync.Mutex

	// moodSnapshotHook 心情快照读取入口；生产走 engine/mood，测试可替换。
	moodSnapshotHook = mood.Snapshot
	// qqProbeHook QQ 适配器只读探测；返回空集合表示适配器不可用。
	qqProbeHook = probeQQTargets
	// newRandHook 随机源；生产按时间播种，测试可替换为固定种子。
	newRandHook = func() *rand.Rand {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
)
