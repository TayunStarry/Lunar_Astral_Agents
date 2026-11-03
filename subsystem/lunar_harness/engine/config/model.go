package config

import "time"

var (
	// AgentEmbeddingModel 核心智能体嵌入模型名称（本地部署固定值，不开放配置）
	AgentEmbeddingModel = "text_embedding"
	// AgentEmbeddingURL 核心智能体嵌入端点：直连 llama.cpp（不经过 Web 服务转发）
	AgentEmbeddingURL = LlamaV1URL()
	// AgentMultimodalModel 核心智能体多模态模型名称（本地部署固定值，不开放配置）
	AgentMultimodalModel = "primary_reasoning"
	// AgentMultimodalURL 核心智能体多模态端点：直连 llama.cpp（不经过 Web 服务转发）
	AgentMultimodalURL = LlamaV1URL()
	// AgentASRModel 核心智能体语音识别模型名称
	AgentASRModel = "audio_transcriber"

	// MmprojModel 多模态投影模型路径
	MmprojModel = LocalDir + "/models/mmproj-Qwen3.GGUF"
	// AsrModel ASR 语音识别模型路径
	AsrModel = LocalDir + "/models/audio_transcriber"
	// DiffusionModel 扩散模型路径
	DiffusionModel = LocalDir + "/models/Qwen3.GGUF"
	// VariationalModel VAE 模型路径
	VariationalModel = LocalDir + "/models/Qwen3.GGUF"
	// PromptAnalysisModel 提示分析模型路径
	PromptAnalysisModel = LocalDir + "/models/Qwen3.GGUF"
	// PromptMmprojModel 多模态投影模型路径（暂不可用）
	PromptMmprojModel = ""
	// RealESRGANModel RealESRGAN 模型路径
	RealESRGANModel = LocalDir + "/models/Qwen3.pth"

	// MusicGenerationModel 音乐生成模型名称（对应 models.ini 的 [music_generation]）
	MusicGenerationModel = "music_generation"
	// MusicGenerationTimeout 音乐模型单次旋律生成超时
	MusicGenerationTimeout = 3 * time.Minute
	// MusicRenderTimeout 单次 fluidsynth 渲染超时（秒）
	MusicRenderTimeout = 90
	// MusicFluidSynth fluidsynth 可执行文件路径
	MusicFluidSynth = LocalDir + "/models/music_generation/fluidsynth/fluidsynth.exe"
	// MusicSoundFont 演奏音色库路径
	MusicSoundFont = LocalDir + "/models/music_generation/soundfonts/general.sf2"
	// MusicOutputDir 演奏音频输出目录（约定 local_data/audios）
	MusicOutputDir = LocalDir + "/audios"
)
