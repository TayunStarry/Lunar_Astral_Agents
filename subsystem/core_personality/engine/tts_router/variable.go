package ttsrouter

// ==== 引擎目录与默认值常量 ====

const (
	// qwenModelDirName Qwen3-TTS 模型目录名（位于 {LocalDir}/models 下）
	qwenModelDirName = "Qwen3-TTS"
	// kokoroModelDirName Kokoro 模型目录名
	kokoroModelDirName = "Kokoro-82M-v1.1-zh"
	// omniModelDirName OmniVoice 模型目录名
	omniModelDirName = "OmniVoice"

	// qwenDefaultRefAudio Qwen 默认参考音频（相对 LocalDir；历史行为沿用）
	qwenDefaultRefAudio = "audios/lunar-template.wav"

	// kokoroOnnxDllName Kokoro 模型目录内的 onnxruntime 运行库
	kokoroOnnxDllName = "onnxruntime.dll"
)

// ==== 闲置卸载守护常量与状态 ====

const (
	// ttsIdleDefaultSeconds 引擎闲置卸载默认阈值：5 分钟无合成调用即卸载模型占用
	ttsIdleDefaultSeconds = 300
	// ttsIdleScanInterval 闲置扫描周期（秒）
	ttsIdleScanInterval = 30
)

var (
	// idleUnloadSeconds 引擎闲置卸载阈值（秒）：启动时由配置 tts.idle_unload_minutes 换算（默认 5 分钟）
	idleUnloadSeconds int64 = ttsIdleDefaultSeconds
	// idleUnloadState 各引擎最近一次被卸载时的 lastUsed 值（引擎名 → 时间戳）。
	// 卸载后 lastUsed 不再变化，据此跳过「已卸载但每轮重复上报」的日志刷屏与重复卸载调用。
	// 仅由 StartIdleUnloader 的单条扫描 goroutine 独占读写，无并发访问。
	idleUnloadState = map[string]int64{}
)
