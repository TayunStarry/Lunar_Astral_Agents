package module

import "sync"

// 全局引擎实例与初始化控制
var (
	// globalEngine 全局 Kokoro TTS 引擎实例
	globalEngine *Engine
	// initOnce 保证引擎只初始化一次
	initOnce sync.Once
	// engineInitMu 保护引擎初始化与获取的并发
	engineInitMu sync.Mutex
	// pronunciationDict 用户读音词典（全局单例，引擎初始化时加载）
	pronunciationDict *PronunciationDict
)

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
)
