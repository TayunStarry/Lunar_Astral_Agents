package config

import "sync"

// Developer 调试模式开关，用于开启调试日志
var Developer = false

// SyncFetchTimeout 模型与宿主内部请求超时时间（秒），默认 300 秒。
var SyncFetchTimeout = 300

// AllowLAN 是否允许局域网访问引擎 HTTP/WS 服务。
var AllowLAN = false

// InsecureTLS 插件沙箱网络是否跳过 TLS 证书校验。
var InsecureTLS = false

// ConsoleLogEnabled 是否把控制台输出镜像落盘到 local_data/logs/console.log。
var ConsoleLogEnabled = false

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
	".html":           "text/html",
	".htm":            "text/html",
	".css":            "text/css",
	".js":             "application/javascript",
	".json":           "application/json",
	".code-workspace": "application/json",
	".png":            "image/png",
	".jpg":            "image/jpeg",
	".jpeg":           "image/jpeg",
	".gif":            "image/gif",
	".svg":            "image/svg+xml",
	".ico":            "image/x-icon",
	".txt":            "text/plain",
	".xml":            "application/xml",
	".pdf":            "application/pdf",
	".woff":           "font/woff",
	".woff2":          "font/woff2",
	".ttf":            "font/ttf",
	".eot":            "application/vnd.ms-fontobject",
	".otf":            "font/otf",
}
