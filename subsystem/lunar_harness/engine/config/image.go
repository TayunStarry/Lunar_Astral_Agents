package config

var (
	// MaxWidth 最大宽度
	MaxWidth = 1920
	// MaxHeight 最大高度
	MaxHeight = 1080
	// JPEGQuality JPEG 压缩质量 (1-100)
	JPEGQuality = 90
	// Format 图片格式 (png, jpg, jpeg)
	Format = "jpg"
	// FfmpegPath ffmpeg 可执行文件路径（空则使用系统 PATH）
	FfmpegPath = ""
	// ImageVRAMGuard 画图前检查可用显存, 低于阈值时先卸载月华模型释放显存
	ImageVRAMGuard = true
	// ImageVRAMGuardMiB 画图前可用显存阈值(MiB)
	ImageVRAMGuardMiB = 8192
	// SDOffloadToCPU sd.cpp 权重驻留内存按需载入显存: auto/always/off
	SDOffloadToCPU = "auto"
	// SDTextEncoderOnCPU sd.cpp 提示词编码器在 CPU 运行(不占显存)
	SDTextEncoderOnCPU = false
	// SDVaeTiling VAE 分片解码: off/always/auto
	SDVaeTiling = "off"
	// SDVaeTileSize VAE 分片尺寸(如 64x64), 空=引擎默认 32x32
	SDVaeTileSize = ""
	// SDVaeTileOverlap VAE 分片重叠比例(0=引擎默认 0.5)
	SDVaeTileOverlap = 0.0
	// SDVaeBackend VAE 解码后端: auto/gpu/cpu
	SDVaeBackend = "auto"
	// SDVaeConvDirect GPU 整帧放不下时改用直接卷积(省显存)
	SDVaeConvDirect = true
	// SDMaxVRAM sd.cpp 显存预算(GiB); 空=自动
	SDMaxVRAM = ""
	// SDSampler 采样器: auto/euler/euler_a/...
	SDSampler = "auto"
	// SDScheduler 调度器: auto/discrete/karras/simple/flux/...
	SDScheduler = "auto"
	// SDFlowShift Flow 模型 shift(0=auto)
	SDFlowShift = 0.0
	// SDFlashAttention 扩散模型使用 flash attention
	SDFlashAttention = true
	// SDSteps 调用方未指定步数时的默认采样步数
	SDSteps = 40
	// SDImageSize 调用方未指定尺寸时的默认边长
	SDImageSize = 1024
	// VideoInputMode 视频解读输入模式: file / frames
	VideoInputMode = "file"
	// ImageInputMode 静态图思维链形态: url / base64
	ImageInputMode = "url"
	// VisionMaxSide 视觉读取副本的最大边长(像素)
	VisionMaxSide = 1080
	// VisionCacheCapMB 视觉副本缓存池容量上限(MB)，0=不限制
	VisionCacheCapMB = 512
)

// ImageVRAMGuardHook 画图前显存守卫钩子，由推理引擎模块注册实现；
// 为 nil 时跳过守卫。
var ImageVRAMGuardHook func() (int, error)
