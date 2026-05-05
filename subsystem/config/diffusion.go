package config

import "flag"

var (
	// VisualEngine 绘图引擎路径，用于图像生成
	VisualEngine = flag.String("visual-engine", *LocalDir+"/models/stable_diffusion.cpp/sd-cli.exe", "绘图引擎路径，用于图像生成")
	// AllowDiffusion 是否允许运行扩散生成机制
	AllowDiffusion = flag.Bool("allow-diffusion", true, "是否允许运行扩散生成机制")
	// DiffusionModel 扩散模型路径，用于图像生成
	DiffusionModel = flag.String("diffusion-model", *LocalDir+"/models/Qwen3.GGUF", "扩散模型路径，用于图像生成")
	// VariationalModel VAE模型路径，用于图像编码与解码
	VariationalModel = flag.String("variational-model", *LocalDir+"/models/Qwen3.GGUF", "VAE模型路径, 用于图像编码与解码")
	// PromptRefineModel 提示精炼模型路径，用于优化图像提示词与负面提示词
	PromptRefineModel = flag.String("prompt_refine-model", *LocalDir+"/models/Qwen3.GGUF", "提示精炼模型路径，用于优化图像提示词与负面提示词")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)
	PromptMmprojModel = flag.String("prompt-mmproj-model", "", "多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)")
)
