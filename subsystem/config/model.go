package config

import "flag"

var (
	// EmbeddingModel 嵌入模型路径，用于文本向量化表示
	EmbeddingModel = flag.String("embedding-model", *LocalDir+"/models/Qwen3.GGUF", "嵌入模型路径, 用于文本向量化表示")
	// MultimodalModel 多模态模型路径，用于图文推理
	MultimodalModel = flag.String("multimodal-model", *LocalDir+"/models/Qwen3.GGUF", "多模态推理模型路径, 用于图文推理")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码
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
