package config

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
)
