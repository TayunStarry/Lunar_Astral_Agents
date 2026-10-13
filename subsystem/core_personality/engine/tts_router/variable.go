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

// ==== 闲置卸载守护常量 ====

const (
	// ttsIdleUnloadSeconds 引擎闲置卸载阈值：5 分钟无合成调用即卸载模型占用
	ttsIdleUnloadSeconds = 300
	// ttsIdleScanInterval 闲置扫描周期
	ttsIdleScanInterval = 30
)
