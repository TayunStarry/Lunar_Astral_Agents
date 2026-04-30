package config

import (
	"flag"
	"sync"
)

// Developer 调试模式开关，用于开启调试日志
var Developer = flag.Bool("developer", false, "启用调试模式, 显示详细日志")

// 系统运行状态变量
var (
	// ModelReady 表示模型是否准备就绪的状态标识，0 可表示未准备就绪。
	ModelReady = 0
	// MaxModelAmount 表示系统支持的最大模型数量，0 可作为初始未设置值。
	MaxModelAmount = 0
	// ServerAddress 存储服务器的IP地址信息
	ServerAddress = []string{}
)

// 模型服务映射与锁
var (
	// ModelPortMap 保存模型名称到其运行端口的映射关系。
	ModelPortMap = make(map[string]int)
	// ModelMapMutex 保护对 ModelPortMap 的并发读写操作。
	ModelMapMutex = sync.RWMutex{}
)

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
	// VS Code 工作区配置文件，本质是 JSON 格式
	".code-workspace": "application/json",
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
