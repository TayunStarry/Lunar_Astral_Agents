package affection

// variable.go 情感认知的锚点、阈值与运行时变量。

import (
	"sync"

	"LunarSubsystem/LunarHarness/engine/memory"
)

const (
	// Module 情感认知日志模块名
	Module = "Affection"
	// stateFile 情感认知状态文件（相对 LocalDir），与心情状态完全分离
	stateFile = "database/life_chain/affection_state.json"
	// initTimeoutSeconds 初始化嵌入的总超时
	initTimeoutSeconds = 20
)

// positiveLabels 正向情感取向锚点（顺序固定，用于极性求和）。
var positiveLabels = []string{"喜欢", "信任", "关心"}

// negativeLabels 负向情感取向锚点（顺序固定，用于极性求和）。
var negativeLabels = []string{"讨厌", "反感", "冷淡"}

// anchorTexts 两极锚点的语义文本；键名即情感取向标签名。
var anchorTexts = map[string]string{
	"喜欢": "我很喜欢这个人，愿意亲近对方",
	"信任": "我信任这个人，愿意和对方说心里话",
	"关心": "我很关心这个人，想照顾对方的感受",
	"讨厌": "我讨厌这个人，不想理对方",
	"反感": "我反感这个人，想和对方保持距离",
	"冷淡": "我对这个人很冷淡，不愿意接近",
}

// 好感度标签阈值（双极轴）。
const (
	strongPositiveThreshold = 0.6
	positiveThreshold       = 0.2
	negativeThreshold       = -0.2
	strongNegativeThreshold = -0.6
)

// defaultLabel 没有足够极性时的中性标签。
const defaultLabel = "平淡"

// affectionAlpha 历史好感度的权重（越大越稳定，越难被单次交互带偏）。
const affectionAlpha = 0.7

// affectionTemperature 极性分布 softmax 温度（越小越接近最强锚点）。
const affectionTemperature = 0.15

var (
	mu          sync.RWMutex
	initMu      sync.Mutex
	state       State
	initialized bool
)

// currentAlpha 允许测试覆盖。
var currentAlpha = affectionAlpha

// currentTemperature 允许测试覆盖。
var currentTemperature = affectionTemperature

// embedDocumentsHook 嵌入入口；测试可替换，生产走记忆库同一嵌入模型。
var embedDocumentsHook = memory.EmbedDocuments
