package wishchain

// variable.go 心愿链路的常量、锚点与运行时变量（业务逻辑不得写在本文件）。

import (
	"regexp"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/memory"
)

const (
	// Module 日志模块名
	Module = "WishChain"
	// stateFile 心愿链路统一持久化文件（相对 LocalDir）
	stateFile = "database/life/wish_chain.json"
	// stateVersion 状态文件版本
	stateVersion = 1
	// initTimeoutSeconds 初始化嵌入的总超时
	initTimeoutSeconds = 20
	// linkLimit 联动记录保留条数
	linkLimit = 128
)

// labels 心情基础情绪词表：顺序固定，用于嵌入锚点、标签匹配与兜底。
//
// 本轮情绪体系重置为以下 11 个锚点。
var labels = []string{
	"开心", "愤怒", "伤心", "好奇", "无聊",
	"失望", "厌倦", "担忧", "恐慌", "满足", "遗憾",
}

// 心情更新参数。
const (
	// defaultLabel 未初始化时的兜底标签
	defaultLabel = "开心"
	// moodAlpha 历史心情在加权平均中的权重（越大越稳定）
	moodAlpha = 0.35
	// moodTemperature 情绪分布 softmax 温度（越小越集中）
	moodTemperature = 0.10
	// moodRoundWindow 心情动态调整使用的最近对话轮数
	moodRoundWindow = 5
)

// 态度（情感认知）锚点。
var (
	// positiveLabels 正向情感取向锚点
	positiveLabels = []string{"喜欢", "信任", "关心"}
	// negativeLabels 负向情感取向锚点
	negativeLabels = []string{"讨厌", "反感", "冷淡"}
)

// anchorTexts 两极锚点的语义文本；键名即情感取向标签名。
var anchorTexts = map[string]string{
	"喜欢": "我很喜欢这个人，愿意亲近对方",
	"信任": "我信任这个人，愿意和对方说心里话",
	"关心": "我很关心这个人，想照顾对方的感受",
	"讨厌": "我讨厌这个人，不想理对方",
	"反感": "我反感这个人，想和对方保持距离",
	"冷淡": "我对这个人很冷淡，不愿意接近",
}

// 态度积分标签阈值（按积分区间切分）。
const (
	// attitudeStrongPositiveThreshold 很喜欢
	attitudeStrongPositiveThreshold = 45
	// attitudePositiveThreshold 喜欢
	attitudePositiveThreshold = 30
	// attitudeStrongNegativeThreshold 讨厌
	attitudeStrongNegativeThreshold = -30
	// attitudeNegativeThreshold 疏离
	attitudeNegativeThreshold = 0
)

// defaultAttitudeLabel 未达到任何阈值时的中性标签。
const defaultAttitudeLabel = "平淡"

// 态度积分参数。
const (
	// attitudeMinPoints 态度积分下限
	attitudeMinPoints = -50
	// attitudeMaxPoints 态度积分上限
	attitudeMaxPoints = 50
	// attitudeDefaultPoints 新对话者的初始态度分
	attitudeDefaultPoints = 20
	// attitudeMaxStep 单次评估最多加 / 减的分数
	attitudeMaxStep = 10
	// attitudeTemperature 极性分布 softmax 温度
	attitudeTemperature = 0.15
	// attitudeRoundWindow 单个用户参与一次评估的对话轮数（最近 5 轮）
	attitudeRoundWindow = 5
)

// 日期与时间解析规则（沿用系统既有的时间归一化能力）。
var (
	cnDateRegex = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
	fullDateRe  = regexp.MustCompile(`^(\d{4})[年\-/.](\d{1,2})[月\-/.](\d{1,2})日?$`)
	yearMonthRe = regexp.MustCompile(`^(\d{4})[年\-/.](\d{1,2})月?$`)
	monthDayRe  = regexp.MustCompile(`^(\d{1,2})月(\d{1,2})日?$`)
	monthOnlyRe = regexp.MustCompile(`^(\d{1,2})月$`)
	yearOnlyRe  = regexp.MustCompile(`^(\d{4})年?$`)
	halfWidthRe = regexp.MustCompile(`[０-９]`)
	dashWidthRe = regexp.MustCompile(`[－—–]`)
)

// timeLayouts 时间归一化布局。
var timeLayouts = []string{
	"2006-01-02T15:04:05", "2006-01-02T15:04", time.RFC3339,
	"2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006/1/2 15:04:05", "2006/1/2 15:04",
}

var (
	// mu 保护整份心愿链路状态
	mu sync.Mutex
	// initMu 串行化嵌入锚点初始化
	initMu sync.Mutex
	// state 内存状态
	state State
	// loaded 是否已从磁盘载入
	loaded bool
	// initialized 心情与态度锚点是否已就绪
	initialized bool
	// lastMoodWindowKey 上次用于心情更新的窗口最新一轮标识，避免重复计算
	lastMoodWindowKey string

	// 以下参数允许测试覆盖。
	currentMoodAlpha           = moodAlpha
	currentMoodTemperature     = moodTemperature
	currentAttitudeTemperature = attitudeTemperature

	// embedDocumentsHook 嵌入入口；测试可替换。
	embedDocumentsHook = memory.EmbedDocuments
)

// ErrCapacity 待实现愿望已达容量上限。
var ErrCapacity = errCapacity{}

type errCapacity struct{}

func (errCapacity) Error() string {
	return "待实现愿望已达上限（最多 16 条）"
}
