package config

import "flag"

var (
	// ==== 核心智能体模型配置（agent，月华） ====
	// 用于主智能体的对话推理、文本向量化、图文理解等

	// AgentEmbeddingModel 核心智能体嵌入模型名称
	AgentEmbeddingModel = flag.String("agent-embedding-model", "system-embedding", "核心智能体嵌入模型名称")
	// AgentEmbeddingURL 核心智能体嵌入服务 API 地址
	AgentEmbeddingURL = flag.String("agent-embedding-url", "http://127.0.0.1:36789/v1", "核心智能体嵌入服务 API 地址")
	// AgentEmbeddingKey 核心智能体嵌入服务 API 密钥
	AgentEmbeddingKey = flag.String("agent-embedding-key", "", "核心智能体嵌入服务 API 密钥")
	// AgentMultimodalModel 核心智能体多模态模型名称
	AgentMultimodalModel = flag.String("agent-multimodal-model", "system-multimodal", "核心智能体多模态模型名称")
	// AgentMultimodalURL 核心智能体多模态服务 API 地址
	AgentMultimodalURL = flag.String("agent-multimodal-url", "http://127.0.0.1:36789/v1", "核心智能体多模态服务 API 地址")
	// AgentMultimodalKey 核心智能体多模态服务 API 密钥
	AgentMultimodalKey = flag.String("agent-multimodal-key", "", "核心智能体多模态服务 API 密钥")

	// ==== 记忆库模型配置（memory） ====
	// 用于记忆库的文本向量化、标签生成等

	// MemoryEmbeddingModel 记忆库嵌入模型名称
	MemoryEmbeddingModel = flag.String("memory-embedding-model", "system-embedding", "记忆库嵌入模型名称, 用于文本向量化")
	// MemoryEmbeddingURL 记忆库嵌入服务 API 地址
	MemoryEmbeddingURL = flag.String("memory-embedding-url", "http://127.0.0.1:36789/v1", "记忆库嵌入服务 API 地址")
	// MemoryEmbeddingKey 记忆库嵌入服务 API 密钥
	MemoryEmbeddingKey = flag.String("memory-embedding-key", "", "记忆库嵌入服务 API 密钥")
	// MemoryMultimodalModel 记忆库多模态模型名称，用于标签生成
	MemoryMultimodalModel = flag.String("memory-multimodal-model", "system-multimodal", "记忆库多模态模型名称, 用于标签生成")
	// MemoryMultimodalURL 记忆库多模态服务 API 地址
	MemoryMultimodalURL = flag.String("memory-multimodal-url", "http://127.0.0.1:36789/v1", "记忆库多模态服务 API 地址")
	// MemoryMultimodalKey 记忆库多模态服务 API 密钥
	MemoryMultimodalKey = flag.String("memory-multimodal-key", "", "记忆库多模态服务 API 密钥")

	// ==== 智能搜索模型配置（search） ====
	// 用于搜索智能体的文本向量化、图文推理等

	// SearchEmbeddingModel 搜索嵌入模型名称
	SearchEmbeddingModel = flag.String("search-embedding-model", "system-embedding", "搜索嵌入模型名称, 用于文本向量化")
	// SearchEmbeddingURL 搜索嵌入服务 API 地址
	SearchEmbeddingURL = flag.String("search-embedding-url", "http://127.0.0.1:36789/v1", "搜索嵌入服务 API 地址")
	// SearchEmbeddingKey 搜索嵌入服务 API 密钥
	SearchEmbeddingKey = flag.String("search-embedding-key", "", "搜索嵌入服务 API 密钥")
	// SearchMultimodalModel 搜索多模态模型名称，用于图文推理
	SearchMultimodalModel = flag.String("search-multimodal-model", "system-multimodal", "搜索多模态模型名称, 用于图文推理")
	// SearchMultimodalURL 搜索多模态服务 API 地址
	SearchMultimodalURL = flag.String("search-multimodal-url", "http://127.0.0.1:36789/v1", "搜索多模态服务 API 地址")
	// SearchMultimodalKey 搜索多模态服务 API 密钥
	SearchMultimodalKey = flag.String("search-multimodal-key", "", "搜索多模态服务 API 密钥")

	// MmprojModel 多模态投影模型路径，用于图像与文本的联合编码
	MmprojModel = flag.String("mmproj-model", *LocalDir+"/models/mmproj-Qwen3.GGUF", "多模态投影模型路径, 用于图像与文本的联合编码")
	// AsrModel ASR语音识别模型路径，用于语音识别
	AsrModel = flag.String("asr-model", *LocalDir+"/models/Qwen3-ASR-0.6B", "ASR模型路径, 用于语音识别")
	// DiffusionModel 扩散模型路径，用于图像生成
	DiffusionModel = flag.String("diffusion-model", *LocalDir+"/models/Qwen3.GGUF", "扩散模型路径, 用于图像生成")
	// VariationalModel VAE模型路径，用于图像编码与解码
	VariationalModel = flag.String("variational-model", *LocalDir+"/models/Qwen3.GGUF", "VAE模型路径, 用于图像编码与解码")
	// PromptAnalysisModel 提示分析模型路径，用于理解图像提示词与负面提示词
	PromptAnalysisModel = flag.String("prompt-analysis-model", *LocalDir+"/models/Qwen3.GGUF", "提示分析模型路径, 用于理解图像提示词与负面提示词")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)
	PromptMmprojModel = flag.String("prompt-mmproj-model", "", "多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)")
	// RealESRGANModel RealESRGAN模型路径，用于图像超分辨率
	RealESRGANModel = flag.String("real-esrgan-model", *LocalDir+"/models/Qwen3.pth", "RealESRGAN模型路径, 用于图像超分辨率")
)
