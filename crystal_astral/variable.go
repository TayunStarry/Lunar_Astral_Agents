package main

import (
	"embed"
	image_server "image/server"
	"screenshot"
	storage "storage/server"
)

// ==== GGUF 值类型常量 ====

const (
	ggufTypeUint8   uint32 = 0
	ggufTypeInt8    uint32 = 1
	ggufTypeUint16  uint32 = 2
	ggufTypeInt16   uint32 = 3
	ggufTypeUint32  uint32 = 4
	ggufTypeInt32   uint32 = 5
	ggufTypeFloat32 uint32 = 6
	ggufTypeBool    uint32 = 7
	ggufTypeString  uint32 = 8
	ggufTypeArray   uint32 = 9
	ggufTypeUint64  uint32 = 10
	ggufTypeInt64   uint32 = 11
	ggufTypeFloat64 uint32 = 12
)

// ==== 全局变量 ====

// EmbeddedFiles 嵌入的静态资源文件系统
//
//go:embed assets/*
var EmbeddedFiles embed.FS

// SystemEndpoints 系统端点列表
var SystemEndpoints = []SystemEndpoint{
	// ==== 应用与资源 ====
	{Path: "/load/application", Handler: loadApplicationHandler, Method: "POST", Description: "加载应用"},
	{Path: "/background", Handler: storage.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},

	// ==== 文件操作 ====
	{Path: "/file/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取"},
	{Path: "/file/write", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/file/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file/list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/file/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/file/preview", Handler: storage.PreviewHandler, Method: "GET", Description: "全局文件预览（图片/视频/文本）"},
	{Path: "/file/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档"},

	// ==== 扩展包管理 ====
	{Path: "/file/package/install", Handler: storage.InstallPackageHandler, Method: "POST", Description: "安装扩展包"},
	{Path: "/file/package/export", Handler: storage.ExportPackageHandler, Method: "POST", Description: "导出扩展包"},
	{Path: "/file/package/delete", Handler: storage.DeletePackageHandler, Method: "POST", Description: "删除扩展包"},
	{Path: "/api/packages", Handler: scanPackagesHandler, Method: "GET", Description: "扫描包目录"},

	// ==== 数据库 ====
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据管理"},
	{Path: "/chromem/init", Handler: storage.ChromemInitHandler, Method: "POST", Description: "向量数据库初始化"},
	{Path: "/chromem/messages", Handler: storage.ChromemMessagesHandler, Method: "POST", Description: "向量数据库消息管理"},
	{Path: "/chromem/rebuild", Handler: storage.ChromemRebuildHandler, Method: "POST", Description: "重建向量数据库文档索引"},
	{Path: "/chromem/stats", Handler: storage.ChromemStatsHandler, Method: "GET", Description: "向量数据库统计信息"},
	{Path: "/chromem/documents", Handler: storage.ChromemDocumentsHandler, Method: "GET", Description: "向量数据库文档列表"},

	// ==== 截图与图像处理 ====
	{Path: "/capture", Handler: screenshot.HandleScreenshot, Method: "POST", Description: "通用截图"},
	{Path: "/capture/display/", Handler: screenshot.HandleScreenshotDisplay, Method: "GET", Description: "屏幕截图"},
	{Path: "/capture/region", Handler: screenshot.HandleScreenshotRegion, Method: "POST", Description: "区域截图"},
	{Path: "/capture/displays", Handler: screenshot.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: screenshot.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	{Path: "/convert/image", Handler: convertImageHandler, Method: "POST", Description: "单张图片格式转换"},
	{Path: "/convert/batch", Handler: batchConvertHandler, Method: "POST", Description: "批量图片格式转换"},
	{Path: "/convert/list", Handler: listImagesHandler, Method: "POST", Description: "列出文件夹中的图片文件"},

	// ==== AI 模型与推理 ====
	{Path: "/proxy/models", Handler: modelsProxyHandler, Method: "POST", Description: "模型查询代理"},
	{Path: "/proxy/chat", Handler: chatProxyHandler, Method: "POST", Description: "对话代理"},
	{Path: "/gguf/metadata", Handler: ggufMetadataHandler, Method: "POST", Description: "GGUF模型元数据解析"},
	{Path: "/generate", Handler: image_server.GenerateHandler, Method: "POST", Description: "图像生成"},
	{Path: "/generate/wait", Handler: image_server.GenerateWaitHandler, Method: "GET", Description: "图像生成等待"},

	// ==== 月华服务 ====
	{Path: "/lunar/check", Handler: yuehuaCheckHandler, Method: "GET", Description: "检测月华服务状态"},
	{Path: "/lunar/start", Handler: yuehuaStartHandler, Method: "POST", Description: "启动月华服务"},
}

// proxyPrefixes 要代理的路径前缀
var proxyPrefixes = []string{"/v1/", "/write/message", "/tts", "/tts/stream", "/ltpx/"}

// supportedFormats 支持的图片格式
var supportedFormats = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
}
