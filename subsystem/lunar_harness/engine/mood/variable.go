package mood

import (
	"sync"

	"LunarSubsystem/LunarHarness/engine/memory"
)

// 5 种基础情绪，顺序固定，用于标签匹配与兜底。
var labels = []string{"开心", "愤怒", "伤心", "好奇", "无聊"}

const (
	// stateFile 心情状态文件（相对 LocalDir）：5 类基准情绪向量 + 当前多维心情
	stateFile = "database/life_chain/mood_state.json"
	// defaultLabel 未初始化时的兜底标签
	defaultLabel = "开心"
	// alpha 当前心情在加权平均中的权重（越大越稳定）
	alpha = 0.7
	// temperature 情绪分布 softmax 温度（越小越集中）
	temperature = 0.15
	// initTimeout 初始化嵌入的总超时
	initTimeoutSeconds = 20
)

var (
	mu          sync.RWMutex
	initMu      sync.Mutex
	state       State
	initialized bool
)

// currentAlpha 允许测试或后续配置覆盖。
var currentAlpha = alpha

// currentTemperature 允许测试或后续配置覆盖。
var currentTemperature = temperature

// embedDocumentsHook 嵌入入口；测试可替换，生产走记忆库同一向量空间。
var embedDocumentsHook = memory.EmbedDocuments
