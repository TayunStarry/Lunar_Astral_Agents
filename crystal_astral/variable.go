package main

import (
	file "LunarSubsystem/FileManager/server"
	image "LunarSubsystem/ImageProcessor/server"
	media "LunarSubsystem/MediaTools/server"
	"embed"
	"sync"
	"syscall"
	"time"
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
// 在 StartServer() 中初始化，供所有前端组件通过 /ws 端点连接（无差别广播）
var StudioHubInstance *StudioHub

// ==== 启动语音（后端直接播放） ====

// startupVoiceMutex 保护启动语音决策的并发读写
var startupVoiceMutex sync.RWMutex

// lastStartupVoice 最近一次启动时的语音决策（由后端直接播放对应语音，前端不再播放）
// 默认 Voice 为空串表示「尚未决策」
var lastStartupVoice = StartupVoice{}

// ==== winmm 播放（后端播放启动语音 WAV，绕开浏览器自动播放限制） ====

// winmmDLL Windows 多媒体库（winmm.dll），提供 PlaySoundW 播放 WAV 音频
var winmmDLL = syscall.NewLazyDLL("winmm.dll")

// procPlaySoundW PlaySoundW 函数句柄：从文件播放 WAV（SND_FILENAME）
var procPlaySoundW = winmmDLL.NewProc("PlaySoundW")

// ==== LTPX 动态工具链（来自包的 AtoA 能力） ====

// ltpToolchainMutex 保护工具链注册表与工具名→包名映射的并发读写
var ltpToolchainMutex sync.RWMutex

// ltpAtoaTools 从包元数据扫描收集的「智能体式」工具链（随包增删动态变化）
var ltpAtoaTools []LTPXRemoteToolDef

// ltpToolPackageMap 工具名 → 提供该工具的包目录名（用于 /ltpx/call 路由到对应包）
var ltpToolPackageMap = map[string]string{}

// ==== LTPX 待定调用（转发前端包执行） ====

// ltpPendingMutex 保护待定调用注册表的并发读写
var ltpPendingMutex sync.Mutex

// ltpPendingCalls 待定调用注册表：request_id → 结果通道（前端包执行完毕回执后解除阻塞）
var ltpPendingCalls = map[string]chan LTPXRemoteCallResponse{}

// ltpCallTimeout 等待前端包执行工具结果的最大时长
const ltpCallTimeout = 120 * time.Second

//==== 音频辅助 ====

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
	{Path: "/file/organize", Handler: file.OrganizeHandler, Method: "POST", Description: "批量文件整理操作"},

	// ==== 扩展包管理 ====
	{Path: "/file/package/install", Handler: file.InstallPackageHandler, Method: "POST", Description: "安装扩展包"},
	{Path: "/file/package/export", Handler: file.ExportPackageHandler, Method: "POST", Description: "导出扩展包"},
	{Path: "/file/package/delete", Handler: file.DeletePackageHandler, Method: "POST", Description: "删除扩展包"},
	{Path: "/file/hash-rename", Handler: file.HashRenameHandler, Method: "POST", Description: "哈希命名（MD5前16位）"},
	{Path: "/api/packages", Handler: scanPackagesHandler, Method: "GET", Description: "扫描包目录"},
	{Path: "/api/module/create", Handler: moduleCreateHandler, Method: "POST", Description: "创建模块（URL/路径/ZIP，支持 Mini-LTP 注入智能体）"},
	{Path: "/api/module/inspect", Handler: moduleInspectHandler, Method: "POST", Description: "检查 HTML 项目内容（README/title/文件清单，供 AI 生成模块元信息）"},

	// ==== 知识库与记忆库 ====
	{Path: "/knowledge/", Handler: file.KnowledgeHandler, Method: "POST", Description: "知识库管理"},
	{Path: "/memory/", Handler: file.MemoryHandler, Method: "ANY", Description: "记忆库（实例初始化/集合管理/消息增删查/文档列表/重建）"},

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

	// ==== 月华服务 ====
	{Path: "/lunar/check", Handler: yuehuaCheckHandler, Method: "GET", Description: "检测月华服务状态"},
	{Path: "/lunar/start", Handler: yuehuaStartHandler, Method: "POST", Description: "启动月华服务"},

	// ==== LTPX 远程（月华调用琉璃）接口 ====
	{Path: "/ltpx/ping", Handler: ltpRemotePingHandler, Method: "GET", Description: "月华探测琉璃是否在线"},
	{Path: "/ltpx/tools", Handler: ltpRemoteToolsHandler, Method: "GET", Description: "月华拉取琉璃工具链（动态扫描包 AtoA 能力）"},
	{Path: "/ltpx/call", Handler: ltpRemoteCallHandler, Method: "POST", Description: "月华调用琉璃工具（转发到前端对应包执行）"},
	{Path: "/ltpx/result", Handler: ltpRemoteResultHandler, Method: "POST", Description: "前端包执行完毕后回执结果"},
	{Path: "/mini-ltp-agent.js", Handler: miniLTPAgentHandler, Method: "GET", Description: "通用页面操作智能体脚本（前端动态注入 Mini-LTP 包）"},
	{Path: "/shared-input.js", Handler: sharedInputHandler, Method: "GET", Description: "统一键鼠操作共享模块（Self-LTP 与 Mini-LTP 智能体共享的页面操作原语）"},
	{Path: "/self-ltp-agent.js", Handler: selfLTPAgentHandler, Method: "GET", Description: "自主页面操作智能体脚本（前端动态注入 Self-LTP 包，自带开始/停止控制面板）"},

	// ==== 引擎消息总线 ====
	{Path: "/write/engine", Handler: StudioEngineHandler, Method: "POST", Description: "引擎/工作室消息（本地 ws 广播）"},
}

// proxyPrefixes 要代理的路径前缀
var proxyPrefixes = []string{"/v1/", "/write/message", "/write/videourl", "/tts", "/tts/stream", "/generate", "/voices", "/dict"}

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
