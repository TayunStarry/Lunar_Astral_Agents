package KokoroTTS

import (
	"sync"
	"sync/atomic"
	"time"
)

// 全局引擎实例与生命周期控制
var (
	// globalEngine 全局 Kokoro TTS 引擎实例
	globalEngine *Engine
	// engineInitMu 引擎生命周期锁：
	//   - 写锁（Lock）串行化加载（InitEngine）与卸载（UnloadEngine）
	//   - 读锁（RLock）在合成期间钉住引擎实例（WithEngine），防止合成中途被卸载
	engineInitMu sync.RWMutex
	// lastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成；供闲置卸载守护判断）
	lastUsedUnix atomic.Int64
	// pronunciationDict 用户读音词典（全局单例，引擎初始化时加载）
	pronunciationDict *PronunciationDict
)

// touchEngine 更新最近合成时间戳（合成入口调用）
func touchEngine() {
	lastUsedUnix.Store(time.Now().Unix())
}

// LastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成）
func LastUsedUnix() int64 {
	return lastUsedUnix.Load()
}

// 模型与音频常量
const (
	// SampleRate 输出音频采样率（24kHz）
	SampleRate = 24000
	// ContextLength 模型最大音素上下文长度（510 + 首尾 pad = 512）
	ContextLength = 510
	// StyleDim 音色向量维度
	StyleDim = 256
	// VoiceRowCount 音色行数（[510, 1, 256]）
	VoiceRowCount = 510
	// MinSpeed / MaxSpeed 语速允许范围
	MinSpeed = 0.5
	MaxSpeed = 2.0
	// DefaultSpeed 默认语速（请求未指定时使用）
	DefaultSpeed = 1.25
	// BatchPauseSeconds 批次之间的静音时长（秒）
	BatchPauseSeconds = 0.2
	// MaxPauseSteps 停顿提示 [•N] 的最大 N（超过按此截断，对应最大停顿 20 秒）
	MaxPauseSteps = 2000
	// RiseFactor 升调提示 [↑] 音高偏移因子（>1 音高升高，约 +1.4 半音）
	RiseFactor = 1.08
	// FallFactor 降调提示 [↓] 音高偏移因子（<1 音高降低，约 -1.3 半音）
	FallFactor = 0.92
)
