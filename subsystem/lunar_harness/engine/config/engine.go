package config

import "flag"

var (
	// InferEngine 推理引擎路径，用于图文推理
	InferEngine = flag.String("infer-engine", *LocalDir+"/models/llama.cpp/llama-server.exe", "推理引擎路径，用于图文推理")
	// VisualEngine 绘图引擎路径，用于图像生成
	VisualEngine = flag.String("visual-engine", *LocalDir+"/models/stable_diffusion.cpp/sd-cli.exe", "绘图引擎路径，用于图像生成")
)
