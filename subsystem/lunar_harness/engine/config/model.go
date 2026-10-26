package config

var (
	// AgentEmbeddingModel 核心智能体嵌入模型名称
	AgentEmbeddingModel = "text_embedding"
	// AgentEmbeddingURL 核心智能体嵌入服务 API 地址
	AgentEmbeddingURL = "http://127.0.0.1:36789/v1"
	// AgentEmbeddingKey 核心智能体嵌入服务 API 密钥
	AgentEmbeddingKey = ""
	// AgentMultimodalModel 核心智能体多模态模型名称
	AgentMultimodalModel = "primary_reasoning"
	// AgentMultimodalURL 核心智能体多模态服务 API 地址
	AgentMultimodalURL = "http://127.0.0.1:36789/v1"
	// AgentMultimodalKey 核心智能体多模态服务 API 密钥
	AgentMultimodalKey = ""
	// AgentASRModel 核心智能体语音识别模型名称
	AgentASRModel = "audio_transcriber"

	// MmprojModel 多模态投影模型路径
	MmprojModel = LocalDir + "/models/mmproj-Qwen3.GGUF"
	// AsrModel ASR 语音识别模型路径
	AsrModel = LocalDir + "/models/Qwen3-ASR-0.6B"
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
)
