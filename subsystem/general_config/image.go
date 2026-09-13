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
)

// ImageVRAMGuardHook 画图前显存守卫钩子，由推理引擎模块（lunar_astral/model/llama）注册实现，
// 图像生成模块在执行扩散生成前调用，返回守卫后的可用显存（MiB，0 表示未知）；
// 为 nil 时跳过守卫。
var ImageVRAMGuardHook func() (int, error)
