package config

import "flag"

var (
	// VisualEngine 绘图引擎路径，用于图像生成
	VisualEngine = flag.String("visual-engine", "./subsystem/engines/sd-cli.exe", "绘图引擎路径，用于图像生成")
	// AllowDiffusion 是否允许运行扩散生成机制
	AllowDiffusion = flag.Bool("allow-diffusion", true, "是否允许运行扩散生成机制")
	// DiffusionModel 扩散模型路径，用于图像生成
	DiffusionModel = flag.String("diffusion-model", "./models/z_image_turbo-Q4_K.gguf", "扩散模型路径，用于图像生成")
	// VariationalModel VAE模型路径，用于图像编码与解码
	VariationalModel = flag.String("variational-model", "./models/diffusion_pytorch_model.safetensors", "VAE模型路径, 用于图像编码与解码")
	// PromptModel 大语言模型路径，用于优化图像提示词与负面提示词
	PromptModel = flag.String("prompt-model", "./models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf", "大语言模型路径，用于优化图像提示词与负面提示词")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)
	PromptMmprojModel = flag.String("prompt-mmproj-model", "", "多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)")
)
