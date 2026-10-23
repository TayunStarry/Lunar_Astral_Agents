package config

var (
	// InferEngine 推理引擎路径，用于图文推理
	InferEngine = LocalDir + "/models/llama.cpp/llama-server.exe"
	// VisualEngine 绘图引擎路径，用于图像生成
	VisualEngine = LocalDir + "/models/stable_diffusion.cpp/sd-cli.exe"
)
