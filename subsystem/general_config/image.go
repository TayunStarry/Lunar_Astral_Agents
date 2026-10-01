package GeneralConfig

import "flag"

var (
	// MaxWidth 最大宽度
	MaxWidth = flag.Int("max-width", 1920, "最大宽度")
	// MaxHeight 最大高度
	MaxHeight = flag.Int("max-height", 1080, "最大高度")
	// JPEGQuality JPEG 压缩质量 (1-100)
	JPEGQuality = flag.Int("jpeg-quality", 90, "JPEG 压缩质量 (1-100)")
	// Format 图片格式 (png, jpg, jpeg)
	Format = flag.String("format", "jpg", "图片格式 (png, jpg, jpeg)")
	// FfmpegPath ffmpeg 可执行文件路径
	FfmpegPath = flag.String("ffmpeg-path", "", "ffmpeg 可执行文件路径，若为空则使用系统 PATH 中的 ffmpeg")
	// ImageVRAMGuard 画图前显存守卫开关：可用显存低于阈值时先卸载本地推理模型释放显存
	ImageVRAMGuard = flag.Bool("image-vram-guard", true, "画图前检查可用显存, 低于阈值时先卸载月华模型释放显存")
	// ImageVRAMGuardMiB 画图前可用显存阈值（MiB），低于该值触发模型卸载，默认 8192（8GB）
	ImageVRAMGuardMiB = flag.Int("image-vram-guard-mib", 8192, "画图前可用显存阈值(MiB), 低于该值先卸载月华模型")
	// SDOffloadToCPU sd.cpp 权重内存卸载模式：auto=卸载月华后显存仍不足时自动启用 / always=始终启用 / off=禁用
	SDOffloadToCPU = flag.String("sd-offload-to-cpu", "auto", "sd.cpp 权重驻留内存按需载入显存: auto/always/off")
	// SDTextEncoderOnCPU 提示词编码器（--llm）在 CPU 上运行，不占显存，速度换显存
	SDTextEncoderOnCPU = flag.Bool("sd-te-on-cpu", false, "sd.cpp 提示词编码器在 CPU 运行(不占显存)")
	// SDVaeTiling VAE 分片解码策略：off=禁用(默认) / always=始终启用 / auto=仅大图或显存吃紧时启用。
	// sd.cpp 的分片默认几何是 tile 32 latent × VAE 16× 压缩 = 512px、overlap 0.5 → 256px 步长，
	// 会在分片角注入硬边纯白方块，因此非必要不启用。
	SDVaeTiling = flag.String("sd-vae-tiling", "off", "VAE 分片解码: off/always/auto")
	// SDVaeTileSize 启用分片时的分片尺寸（latent 单元，如 64x64）；空=sd.cpp 默认 32x32
	SDVaeTileSize = flag.String("sd-vae-tile-size", "", "VAE 分片尺寸(如 64x64), 空=引擎默认 32x32")
	// SDVaeTileOverlap 启用分片时的分片重叠比例；0=sd.cpp 默认 0.5
	SDVaeTileOverlap = flag.Float64("sd-vae-tile-overlap", 0, "VAE 分片重叠比例(0=引擎默认 0.5)")
	// SDMaxVRAM 交给 sd.cpp 图切割执行的显存预算（GiB，支持 cuda0=9 形式）；空=不传，由自动管理决策
	SDMaxVRAM = flag.String("sd-max-vram", "", "sd.cpp 显存预算(GiB, 如 9 或 cuda0=9); 空=自动")
	// SDSampler 采样器：auto=不传参由 sd.cpp 按模型选择（本模型为 euler）
	SDSampler = flag.String("sd-sampler", "auto", "采样器: auto/euler/euler_a/heun/dpm++2m/...")
	// SDScheduler 调度器：auto=不传参由 sd.cpp 按模型选择（本模型为 flux）
	SDScheduler = flag.String("sd-scheduler", "auto", "调度器: auto/discrete/karras/simple/flux/...")
	// SDFlowShift Flow 模型 shift 值；0=auto（由 sd.cpp 按分辨率动态选择）
	SDFlowShift = flag.Float64("sd-flow-shift", 0, "Flow 模型 shift(0=auto)")
	// SDFlashAttention 扩散模型使用 flash attention（速度换数值精度，排查方块伪影时可关闭对比）
	SDFlashAttention = flag.Bool("sd-diffusion-fa", true, "扩散模型使用 flash attention")
	// SDSteps 调用方未指定步数时的默认采样步数（Qwen-Image 2.1 官方默认 40）
	SDSteps = flag.Int("sd-steps", 40, "默认采样步数(调用方未指定时)")
	// SDImageSize 调用方未指定尺寸时的默认边长（像素）；实际会归一化为 32 的倍数
	SDImageSize = flag.Int("sd-image-size", 1024, "默认图片边长(调用方未指定时)")
	// VideoInputMode 视频/动态图解读的输入模式：
	// file=写媒体目录后以 file:// 引用交 llama-server 抽帧（默认，行为与历史一致）；
	// frames=本地抽帧为图片序列帧（base64）后以多个 image_url 提交，供不支持 file:// 的架构使用
	VideoInputMode = flag.String("video-input", "file", "视频解读输入模式: file(file://) / frames(图片序列帧)")
)

// ImageVRAMGuardHook 画图前显存守卫钩子，由推理引擎模块（lunar_astral/model/llama）注册实现，
// 图像生成模块在执行扩散生成前调用，返回守卫后的可用显存（MiB，0 表示未知）；
// 为 nil 时跳过守卫。
var ImageVRAMGuardHook func() (int, error)
