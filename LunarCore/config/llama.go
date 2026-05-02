package config

import "flag"

var (
	// InferEngine 推理引擎路径，用于图文推理
	InferEngine = flag.String("infer-engine", *LocalDir+"/models/llama.cpp/llama-server.exe", "推理引擎路径，用于图文推理")
	// ModelPort 模型服务的基础端口号，用于分配模型运行端口
	ModelPort = flag.Int("model-port", *BasicPort+1, "模型服务的基础端口号，用于分配模型运行端口")
	// AllowMultimodal 是否允许加载多模态模型进行推理
	AllowMultimodal = flag.Bool("allow-multimodal", true, "是否允许加载多模态模型进行推理")
	// EmbeddingModel 嵌入模型路径，用于文本向量化表示
	EmbeddingModel = flag.String("embedding-model", *LocalDir+"/models/Qwen3-Embedding-0.6B-Q8_0.gguf", "嵌入模型路径，用于文本向量化表示")
	// MultimodalModel 多模态模型路径，用于图文推理
	MultimodalModel = flag.String("multimodal-model", *LocalDir+"/models/Qwen3-VL-8B-Instruct-Q4_K_M.gguf", "多模态推理模型路径，用于图文推理")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码
	MmprojModel = flag.String("mmproj-model", *LocalDir+"/models/mmproj-Qwen3-VL-8B-Instruct-F16.gguf", "多模态投影模型路径，用于图像与文本的联合编码")
)
