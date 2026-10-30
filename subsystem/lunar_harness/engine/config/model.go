package config

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
