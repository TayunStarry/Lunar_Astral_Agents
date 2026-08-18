package main

import (
	file "LunarSubsystem/FileManager/server"
	image "LunarSubsystem/ImageProcessor/server"
	media "LunarSubsystem/MediaTools/server"
	"embed"
)

// ==== 记忆库自动初始化默认值 ====
// 与 lunar_astral 的 TypeScript 默认值保持一致（见 server_side/config/global.ts）
const (
	defaultMemoryModelName  = "system-embedding" // 默认嵌入模型名
	defaultMemoryCollection = "lunar_messages"   // memory_store 前端操作的固定集合名
)

// ==== 全局变量 ====

// EmbeddedFiles 嵌入的静态资源文件系统
//
//go:embed assets/*
var EmbeddedFiles embed.FS

// StudioHubInstance 工作室 WebSocket 集线器全局实例
// 在 StartServer() 中初始化，供所有前端组件通过 /ws/studio 端点连接
var StudioHubInstance *StudioHub

// animCache 动画列表缓存（从引擎 animation_list 消息中提取的动作定义）
// 由 StudioHub.Run() 中 cacheAnimationList() 更新，由 HandleGetAnimations 读取
var animCache = &AnimationListCache{}

// SystemEndpoints 系统端点列表
var SystemEndpoints = []SystemEndpoint{
	// ==== 应用与资源 ====
	{Path: "/load/application", Handler: loadApplicationHandler, Method: "POST", Description: "加载应用"},
	{Path: "/background", Handler: file.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},

	// ==== 文件操作 ====
	{Path: "/file/read/", Handler: file.ReadHandler, Method: "GET", Description: "文件读取"},
	{Path: "/file/write", Handler: file.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/file/delete/", Handler: file.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file/list/", Handler: file.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/file/download/", Handler: file.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/file/preview", Handler: file.PreviewHandler, Method: "GET", Description: "全局文件预览（图片/视频/文本）"},
	{Path: "/file/archive", Handler: file.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/file/archive/create", Handler: file.CreateZipHandler, Method: "POST", Description: "服务端压缩（支持文件夹）"},
	{Path: "/file/archive/metadata", Handler: file.ZipMetadataHandler, Method: "POST", Description: "ZIP压缩包元数据查询"},
	{Path: "/file/archive/extract", Handler: file.ExtractZipHandler, Method: "POST", Description: "ZIP解压到服务器目录"},
	{Path: "/file/move", Handler: file.MoveHandler, Method: "POST", Description: "文件移动操作（含冲突处理）"},

	// ==== 扩展包管理 ====
	{Path: "/file/package/install", Handler: file.InstallPackageHandler, Method: "POST", Description: "安装扩展包"},
	{Path: "/file/package/export", Handler: file.ExportPackageHandler, Method: "POST", Description: "导出扩展包"},
	{Path: "/file/package/delete", Handler: file.DeletePackageHandler, Method: "POST", Description: "删除扩展包"},
	{Path: "/file/hash-rename", Handler: file.HashRenameHandler, Method: "POST", Description: "哈希命名（MD5前16位）"},
	{Path: "/api/packages", Handler: scanPackagesHandler, Method: "GET", Description: "扫描包目录"},

	// ==== 知识库与记忆库 ====
	{Path: "/knowledge/", Handler: file.KnowledgeHandler, Method: "POST", Description: "知识库管理"},
	{Path: "/memory/", Handler: file.MemoryHandler, Method: "ANY", Description: "记忆库（实例初始化/集合管理/消息增删查/文档列表/重建）"},

	// ==== 文件整理 ====
	{Path: "/file/organize", Handler: file.OrganizeHandler, Method: "POST", Description: "批量文件整理操作"},

	// ==== 截图与图像处理 ====
	{Path: "/capture", Handler: image.HandleCapture, Method: "ANY", Description: "统一截图（auto/window/fullscreen/display/region）"},
	{Path: "/keyframe", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频关键帧提取"},
	{Path: "/capture/displays", Handler: image.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: image.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	{Path: "/convert/image", Handler: media.ConvertImageHandler, Method: "POST", Description: "单张图片格式转换"},
	{Path: "/convert/batch", Handler: media.BatchConvertHandler, Method: "POST", Description: "批量图片格式转换"},
	{Path: "/convert/list", Handler: media.ListImagesHandler, Method: "POST", Description: "列出文件夹中的图片文件"},

	// ==== AI 模型与推理 ====
	{Path: "/proxy/models", Handler: modelsProxyHandler, Method: "POST", Description: "模型查询代理"},
	{Path: "/proxy/chat", Handler: chatProxyHandler, Method: "POST", Description: "对话代理"},
	{Path: "/gguf/metadata", Handler: media.GGUFMetadataHandler, Method: "POST", Description: "GGUF模型元数据解析"},
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图像生成"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "图像生成等待"},

	// ==== 月华服务 ====
	{Path: "/lunar/check", Handler: yuehuaCheckHandler, Method: "GET", Description: "检测月华服务状态"},
	{Path: "/lunar/start", Handler: yuehuaStartHandler, Method: "POST", Description: "启动月华服务"},

	// ==== 引擎命令桥接 ====
	{Path: "/api/engine/command", Handler: HandleEngineCommand, Method: "POST", Description: "智能体引擎命令转发"},
	{Path: "/api/engine/animations", Handler: HandleGetAnimations, Method: "GET", Description: "查询引擎可用动作列表"},
}

// proxyPrefixes 要代理的路径前缀
var proxyPrefixes = []string{"/v1/", "/write/message", "/tts", "/tts/stream", "/ltpx/"}

// fileCategoryMap 文件扩展名到分类的映射
var fileCategoryMap = map[string]string{
	// 文本文件
	".txt": "text", ".md": "text", ".log": "text", ".csv": "text",
	".json": "text", ".xml": "text", ".yaml": "text", ".yml": "text",
	".toml": "text", ".ini": "text", ".cfg": "text", ".conf": "text",
	".html": "text", ".css": "text", ".js": "text", ".ts": "text",
	".go": "code", ".py": "code", ".java": "code", ".c": "code",
	".cpp": "code", ".h": "code", ".rs": "code", ".rb": "code",
	".sh": "code", ".bat": "code", ".ps1": "code",
	// 图片文件
	".png": "image", ".jpg": "image", ".jpeg": "image", ".webp": "image",
	".gif": "image", ".svg": "image", ".bmp": "image", ".ico": "image",
	".tiff": "image", ".tif": "image", ".avif": "image",
	// 视频文件
	".mp4": "video", ".webm": "video", ".avi": "video", ".mov": "video",
	".mkv": "video", ".wmv": "video", ".flv": "video", ".m4v": "video",
	".mpg": "video", ".mpeg": "video",
	// 音频文件
	".mp3": "audio", ".wav": "audio", ".flac": "audio", ".aac": "audio",
	".ogg": "audio", ".wma": "audio", ".m4a": "audio",
	// 压缩文件
	".zip": "archive", ".rar": "archive", ".7z": "archive", ".tar": "archive",
	".gz": "archive", ".bz2": "archive", ".xz": "archive",
}
