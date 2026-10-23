package broadcaster

import (
	"sync"
	"sync/atomic"

	KokoroTTS "LunarSubsystem/LunarHarness/engine/kokoro_tts"
	OmniVoiceTTS "LunarSubsystem/LunarHarness/engine/omnivoice_tts"
)

// ==== 引擎目录与槽位常量 ====

const (
	// EngineQwen Qwen3-TTS 引擎名（TTSRequest.engine 取值；tts.engine 配置默认值）
	EngineQwen = "Qwen"
	// EngineKokoro Kokoro 引擎名
	EngineKokoro = "Kokoro"
	// EngineOmniVoice OmniVoice 引擎名
	EngineOmniVoice = "OmniVoice"
)

// 模型目录名（位于 {LocalDir}/models 下）
const (
	qwenModelDirName   = "Qwen3-TTS"
	kokoroModelDirName = "Kokoro-82M-v1.1-zh"
	omniModelDirName   = "OmniVoice"
)

// qwenDefaultRefAudio Qwen 默认参考音频（相对 LocalDir；历史行为沿用）
const qwenDefaultRefAudio = "audios/lunar-template.wav"

// kokoroOnnxDllName Kokoro 模型目录内的 onnxruntime 运行库
const kokoroOnnxDllName = "onnxruntime.dll"

// logModule 统一日志模块名
const logModule = "Broadcaster"

// ==== 闲置卸载守护常量 ====

const (
	// idleUnloadDefaultSeconds 槽位闲置卸载默认阈值：5 分钟无活动即卸载在位引擎
	idleUnloadDefaultSeconds = 300
	// idleScanInterval 闲置扫描周期（秒）
	idleScanInterval = 30
)

// ==== 槽位核心状态 ====

var (
	// slotMu 槽位操作互斥锁：全程串行化「加载 / 卸载 / 切换 / 合成」四类槽位操作。
	// 合成全程持锁——切换请求在合成进行中到达时自然排队等待合成完毕，
	// 随后执行「卸载在位 → 加载目标」的切换序列，与槽位切换语义一致；
	// 各引擎内部的生命周期锁（等待引擎内合成完成）是第二道保险。
	slotMu sync.Mutex

	// slotStateVal 槽位状态机（原子：/broadcaster/status 零锁观测）
	slotStateVal atomic.Int32

	// slotEngineName 当前在位引擎名（原子："" = 空槽；slotMu 写、任意读）
	slotEngineName atomic.Value // string

	// slotLastUseUnix 槽位最近一次活动（引擎加载成功或合成完成）的 unix 秒。
	// 以槽位为单位而非逐引擎时间戳：引擎重载后旧时间戳不应把新槽位立刻判为闲置。
	slotLastUseUnix atomic.Int64

	// switchCount 槽位切换累计次数（含空槽 → 首次加载）
	switchCount atomic.Int64
)

// 槽位状态枚举（slotStateVal 存储）
const (
	stateUnloaded int32 = iota
	stateLoading
	stateReady
	stateSynthesizing
	stateUnloading
)

// ==== 引擎能力矩阵（静态声明，/broadcaster/capabilities 原样输出） ====

var engineCapabilities = []EngineCapability{
	{
		Engine:        EngineQwen,
		Label:         "Qwen3-TTS 参考音频声音克隆（采样控制精细）",
		RefAudioClone: true,
		Sampling:      true,
		SynthCache:    true,
		SampleRate:    24000,
	},
	{
		Engine:       EngineKokoro,
		Label:        "Kokoro 内置音色选择与多音色融合（中英混读，响应携带音素）",
		VoiceSelect:  true,
		VoiceMix:     true,
		Speed:        true,
		Lang:         true,
		Phonemes:     true,
		SampleRate:   KokoroTTS.SampleRate,
		AuxEndpoints: []string{"/broadcaster/kokoro/voices", "/broadcaster/kokoro/dict", "/broadcaster/kokoro/dict/guess"},
	},
	{
		Engine:         EngineOmniVoice,
		Label:          "OmniVoice 属性关键词设计音色 / 参考音频+转写克隆",
		RefAudioClone:  true,
		RefText:        true,
		Lang:           true,
		Instruct:       true,
		Seed:           true,
		PostprocToggle: true,
		SampleRate:     OmniVoiceTTS.SampleRate,
		AuxEndpoints:   []string{"/broadcaster/omnivoice/languages"},
	},
}
