package main

// ==== 常量与默认配置 ====

// SourceDirName 内容仓库目录名（创建在目标目录下，按内容哈希存储实体文件）
const SourceDirName = "source_data"

// LinkTempSuffix 硬链接替换时的临时链接后缀
const LinkTempSuffix = ".wh_tmp"

// DefaultExts 默认处理的图片/视频扩展名（小写，含点）
var DefaultExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".bmp": true, ".webm": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".flv": true,
}
