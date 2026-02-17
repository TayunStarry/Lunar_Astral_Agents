package config

import (
	"flag" // 用于解析命令行参数
	"sync" // 用于提供同步原语，如互斥锁
)

// ==================== 系统核心配置 ====================

// 系统运行状态变量
var (
	// ModelReady 表示模型是否准备就绪的状态标识，0 可表示未准备就绪。
	ModelReady = 0
	// MaxModelAmount 表示系统支持的最大模型数量，0 可作为初始未设置值。
	MaxModelAmount = 0
)

// 模型服务映射与锁
var (
	// ModelPortMap 保存模型名称到其运行端口的映射关系。
	ModelPortMap = make(map[string]int)
	// ModelMapMutex 保护对 ModelPortMap 的并发读写操作。
	ModelMapMutex = sync.RWMutex{}
)

// ==================== 命令行参数配置 ====================

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口，用户可通过此端口访问客户端界面")
	// MaxPort 系统Web服务的最大监听端口，界定了系统Web服务的端口范围
	MaxPort = flag.Int("max-port", *BasicPort+15, "系统Web服务的最大监听端口，界定了系统Web服务的端口范围")
	// MinPort 系统Web服务的最小监听端口，界定了系统Web服务的端口范围
	MinPort = flag.Int("min-port", *BasicPort-5, "系统Web服务的最小监听端口，界定了系统Web服务的端口范围")
	// DevMode  启用调试模式，显示详细日志且不自动打开Web界面
	DevMode = flag.Bool("dev-mode", false, "启用调试模式，显示详细日志且不自动打开Web界面")
	// ClearPort 启动时自动释放被占用的端口
	ClearPort = flag.Bool("clear-port", true, "启动时自动释放被占用的端口")
	// CertFile  证书文件路径，用于HTTPS加密通信
	CertFile = flag.String("cert-file", LocalDir+"/certs/localhost.pem", "证书文件路径，用于HTTPS加密通信")
	// KeyFile  私钥文件路径，用于HTTPS加密通信
	KeyFile = flag.String("key-file", LocalDir+"/certs/localhost-key.pem", "私钥文件路径，用于HTTPS加密通信")
	// Database  SQLite数据库文件路径，用于存储系统数据
	Database = flag.String("database", LocalDir+"/resources/SQLite.db", "SQLite数据库文件路径，用于存储系统数据")
)

var (
	// InferEngine 推理引擎路径，用于图文推理
	InferEngine = flag.String("infer-engine", "./subsystem/engines/llama-server.exe", "推理引擎路径，用于图文推理")
	// VisualEngine 绘图引擎路径，用于图像生成
	VisualEngine = flag.String("visual-engine", "./subsystem/engines/sd-cli.exe", "绘图引擎路径，用于图像生成")
	// ModelPort 模型服务的基础端口号，用于分配模型运行端口
	ModelPort = flag.Int("model-port", *BasicPort+1, "模型服务的基础端口号，用于分配模型运行端口")
	// AllowMultimodal 是否允许加载多模态模型进行推理
	AllowMultimodal = flag.Bool("allow-multimodal", true, "是否允许加载多模态模型进行推理")
	// EmbeddingModel 嵌入模型路径，用于文本向量化表示
	EmbeddingModel = flag.String("embedding-model", "./models/Qwen3-Embedding-0.6B-Q8_0.gguf", "嵌入模型路径，用于文本向量化表示")
	// MultimodalModel 多模态模型路径，用于图文推理
	MultimodalModel = flag.String("multimodal-model", "./models/Qwen3-VL-8B-Instruct-Q4_K_M.gguf", "多模态推理模型路径，用于图文推理")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码
	MmprojModel = flag.String("mmproj-model", "./models/mmproj-Qwen3-VL-8B-Instruct-F16.gguf", "多模态投影模型路径，用于图像与文本的联合编码")
)

var (
	// AllowDiffusion 是否启用灵绘坊
	AllowDiffusion = flag.Bool("allow-diffusion", true, "是否启用灵绘坊")
	// DiffusionModel 扩散模型路径，用于图像生成
	DiffusionModel = flag.String("diffusion-model", "./models/z_image_turbo-Q4_K.gguf", "扩散模型路径，用于图像生成")
	// VariationalModel VAE模型路径，用于图像编码与解码
	VariationalModel = flag.String("variational-model", "./models/diffusion_pytorch_model.safetensors", "VAE模型路径，用于图像编码与解码")
	// PromptModel 大语言模型路径，用于优化图像提示词与负面提示词
	PromptModel = flag.String("prompt-model", "./models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf", "大语言模型路径，用于优化图像提示词与负面提示词")
	// PromptMmprojModel 多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)
	PromptMmprojModel = flag.String("prompt-mmproj-model", "", "多模态投影模型路径，用于图像与文本的联合编码(因版本原因, 暂不可用)")
)

var (
	// MaxWidth 最大宽度
	MaxWidth = flag.Int("max-width", 1920, "最大宽度")
	// MaxHeight 最大高度
	MaxHeight = flag.Int("max-height", 1080, "最大高度")
	// JPEGQuality JPEG 压缩质量 (1-100)
	JPEGQuality = flag.Int("jpeg-quality", 80, "JPEG 压缩质量 (1-100)")
	// Format 图片格式 (png, jpg, jpeg)
	Format = flag.String("format", "png", "图片格式 (png, jpg, jpeg)")
)
