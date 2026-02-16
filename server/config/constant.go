package config

// LocalDir 本地数据目录，用于存储模型文件等本地数据
const LocalDir = "local_data"

// 定义文件扩展名与 MIME 类型的映射关系
var MimeMap = map[string]string{
	// HTML 文件的 MIME 类型
	".html": "text/html",
	// HTML 短扩展名文件的 MIME 类型
	".htm": "text/html",
	// CSS 文件的 MIME 类型
	".css": "text/css",
	// JavaScript 文件的 MIME 类型
	".js": "application/javascript",
	// JSON 文件的 MIME 类型
	".json": "application/json",
	// PNG 图片文件的 MIME 类型
	".png": "image/png",
	// JPG 图片文件的 MIME 类型
	".jpg": "image/jpeg",
	// JPEG 图片文件的 MIME 类型
	".jpeg": "image/jpeg",
	// GIF 图片文件的 MIME 类型
	".gif": "image/gif",
	// SVG 图片文件的 MIME 类型
	".svg": "image/svg+xml",
	// ICO 图标文件的 MIME 类型
	".ico": "image/x-icon",
	// 文本文件的 MIME 类型
	".txt": "text/plain",
	// XML 文件的 MIME 类型
	".xml": "application/xml",
	// PDF 文件的 MIME 类型
	".pdf": "application/pdf",
	// WOFF 字体文件的 MIME 类型
	".woff": "font/woff",
	// WOFF2 字体文件的 MIME 类型
	".woff2": "font/woff2",
	// TTF 字体文件的 MIME 类型
	".ttf": "font/ttf",
	// EOT 字体文件的 MIME 类型
	".eot": "application/vnd.ms-fontobject",
	// OTF 字体文件的 MIME 类型
	".otf": "font/otf",
}
