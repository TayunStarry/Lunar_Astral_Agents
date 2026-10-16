package server

import (
	"CorePersonality/engine/kokoro_tts"
	"CorePersonality/engine/omnivoice_tts"
	"CorePersonality/engine/tts_router"
	"CorePersonality/personality/crystal"
	"CorePersonality/personality/lunar"
	"CorePersonality/personality/lunar/model/llama"
	file "LunarSubsystem/FileManager/server"
	media "LunarSubsystem/MediaTools/server"
	image "LunarSubsystem/MultimodalAnalysis/server"
	"net/http"
)

// httpMux HTTP 路由复用器实例（Start 中创建）
var httpMux *http.ServeMux

// SystemEndpoints 统一路由注册表
var SystemEndpoints = []SystemEndpoint{
	// ==================== 宿主平台（自述 / 日志 / 窗口） ====================
	{Path: "/api-docs", Handler: docsHandler, Method: "GET", Description: "服务自述文档"},
	{Path: "/logs", Handler: crystal.LogsHandler, Method: "GET", Description: "后端最近日志（limit 可限制条数，最多 1000 条）"},
	{Path: "/window/focus", Handler: WindowFocusHandler, Method: "POST", Description: "置前指定桌面窗口（id: lunar/crystal）"},

	// ==================== 引擎总线与消息队列 ====================
	{Path: "/write/engine", Handler: EngineMessageHandler, Method: "POST", Description: "引擎/前端系统总线（反射观测 + 原样透传广播）"},
	{Path: "/write/message", Handler: lunar.MessageBatchHandler, Method: "POST", Description: "消息写入队列（文本/多模态内容数组/视频音频URL内容项）"},

	// ==================== 应用与资源 ====================
	{Path: "/background", Handler: file.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},
	{Path: "/load/application", Handler: crystal.LoadApplicationHandler, Method: "POST", Description: "加载应用"},

	// ==================== 文件操作 ====================
	{Path: "/file/write", Handler: file.SaveHandler, Method: "POST", Description: "文件保存操作"},
	{Path: "/file/read/", Handler: file.ReadHandler, Method: "GET", Description: "文件读取操作"},
	{Path: "/file/delete/", Handler: file.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file/list/", Handler: file.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/file/download/", Handler: file.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/file/preview", Handler: file.PreviewHandler, Method: "GET", Description: "全局文件预览（图片/视频/文本）"},
	{Path: "/file/move", Handler: file.MoveHandler, Method: "POST", Description: "文件移动操作（含冲突处理）"},
	{Path: "/file/organize", Handler: file.OrganizeHandler, Method: "POST", Description: "批量文件整理操作"},
	{Path: "/file/hash-rename", Handler: file.HashRenameHandler, Method: "POST", Description: "哈希命名（MD5前16位）"},

	// ==================== 文件归档（ZIP） ====================
	{Path: "/file/archive", Handler: file.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/file/archive/create", Handler: file.CreateZipHandler, Method: "POST", Description: "服务端压缩（支持文件夹）"},
	{Path: "/file/archive/metadata", Handler: file.ZipMetadataHandler, Method: "POST", Description: "ZIP压缩包元数据查询"},
	{Path: "/file/archive/extract", Handler: file.ExtractZipHandler, Method: "POST", Description: "ZIP解压到服务器目录"},

	// ==================== 扩展包与模块 ====================
	{Path: "/file/package/install", Handler: file.InstallPackageHandler, Method: "POST", Description: "安装扩展包"},
	{Path: "/file/package/export", Handler: file.ExportPackageHandler, Method: "POST", Description: "导出扩展包"},
	{Path: "/file/package/delete", Handler: file.DeletePackageHandler, Method: "POST", Description: "删除扩展包"},
	{Path: "/api/packages", Handler: crystal.ScanPackagesHandler, Method: "GET", Description: "扫描包目录"},
	{Path: "/api/module/create", Handler: crystal.ModuleCreateHandler, Method: "POST", Description: "创建模块（URL/路径/ZIP，支持 Mini-LTP 注入智能体）"},
	{Path: "/api/module/inspect", Handler: crystal.ModuleInspectHandler, Method: "POST", Description: "检查 HTML 项目内容（供 AI 生成模块元信息）"},

	// ==================== 控制台数据导出 ====================
	// TUI「数据导出」调用：把原始日志 / 智能体上下文落盘为 .log / .json，便于离线排查
	{Path: "/debug/dump/logs", Handler: DataDumpLogsHandler, Method: "POST", Description: "导出原始日志为 JSON Lines（.log）"},
	{Path: "/debug/dump/context", Handler: DataDumpContextHandler, Method: "POST", Description: "导出月华主上下文或某个子智能体的上下文为 JSON（.json）"},

	// ==================== 知识库与记忆库 ====================
	{Path: "/knowledge/", Handler: file.KnowledgeHandler, Method: "POST", Description: "知识库管理"},
	{Path: "/memory/", Handler: file.MemoryHandler, Method: "ANY", Description: "记忆库（实例初始化/集合管理/消息增删查/文档列表/重建）"},

	// ==================== 截图与图像处理 ====================
	{Path: "/capture", Handler: image.HandleCapture, Method: "ANY", Description: "统一截图（auto/window/fullscreen/display/region）"},
	{Path: "/capture/displays", Handler: image.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: image.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	{Path: "/keyframe", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频关键帧提取"},
	{Path: "/convert/image", Handler: media.ConvertImageHandler, Method: "POST", Description: "单张图片格式转换"},
	{Path: "/convert/batch", Handler: media.BatchConvertHandler, Method: "POST", Description: "批量图片格式转换"},
	{Path: "/convert/list", Handler: media.ListImagesHandler, Method: "POST", Description: "列出文件夹中的图片文件"},
	{Path: "/gguf/metadata", Handler: media.GGUFMetadataHandler, Method: "POST", Description: "GGUF模型元数据解析"},

	// ==================== 图片生成 ====================
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成服务"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成结果"},

	// ==================== 模型服务与显存守卫 ====================
	// llama.cpp 本地代理（同源 /v1，端口随 BasicPort）+ 显存守卫（阈值触发卸载已加载模型）
	{Path: "/v1/", Handler: llama.ProxyHandler, Method: "ANY", Description: "llama.cpp 代理接口"},
	{Path: "/vram/guard", Handler: llama.VRAMGuardHandler, Method: "POST", Description: "显存守卫（可用显存低于阈值时卸载已加载模型，一并释放 KV 缓存）"},
	{Path: "/vram/unload", Handler: llama.VRAMUnloadHandler, Method: "POST", Description: "定向卸载（卸载指定 ID 的已加载模型）"},

	// ==================== 代理请求 ====================
	{Path: "/proxy", Handler: lunar.ProxyHandler, Method: "POST", Description: "代理访问服务"},
	{Path: "/proxy/models", Handler: crystal.ModelsProxyHandler, Method: "POST", Description: "模型查询代理"},
	{Path: "/proxy/chat", Handler: crystal.ChatProxyHandler, Method: "POST", Description: "对话代理"},

	// ==================== 语音合成（统一路由 + 引擎专用端点） ====================
	// /tts 优先按请求 engine 覆写、缺省回退 tts.engine 配置分发；明确锁定引擎的场景用 /qwen、/kokoro、/omnivoice 专用端点
	{Path: "/tts", Handler: ttsrouter.TTSDispatchHandler, Method: "POST", Description: "TTS语音合成（统一参数样式，按请求 engine 覆写或 tts.engine 配置分发：Qwen/Kokoro/OmniVoice）"},
	{Path: "/qwen/tts", Handler: ttsrouter.QwenTTSHandler, Method: "POST", Description: "Qwen 语音合成（参考音频声音克隆；强制 Qwen 引擎，不随 tts.engine 配置漂移）"},
	{Path: "/qwen/health", Handler: ttsrouter.QwenHealthHandler, Method: "GET", Description: "Qwen 引擎健康检查（加载状态与最近合成时间）"},
	{Path: "/kokoro/tts", Handler: crystal.KokoroHandler(KokoroTTS.TTSHandler), Method: "POST", Description: "Kokoro 语音合成（支持 voice 单音色与 mix 多音色混合）"},
	{Path: "/kokoro/voices", Handler: crystal.KokoroHandler(KokoroTTS.VoicesHandler), Method: "GET", Description: "Kokoro 音色列表"},
	{Path: "/kokoro/dict", Handler: crystal.KokoroHandler(KokoroTTS.DictHandler), Method: "GET/POST/DELETE", Description: "Kokoro 读音词典管理"},
	{Path: "/kokoro/dict/guess", Handler: crystal.KokoroHandler(KokoroTTS.GuessDictHandler), Method: "GET", Description: "Kokoro 读音查询"},
	{Path: "/kokoro/health", Handler: crystal.KokoroHandler(KokoroTTS.HealthHandler), Method: "GET", Description: "Kokoro 引擎健康检查"},
	{Path: "/omnivoice/tts", Handler: crystal.OmniVoiceHandler(OmniVoiceTTS.TTSHandler), Method: "POST", Description: "OmniVoice 语音合成（ref_audio+ref_text 声音克隆 / instruct 属性设计）"},
	{Path: "/omnivoice/langs", Handler: crystal.OmniVoiceHandler(OmniVoiceTTS.LanguagesHandler), Method: "GET", Description: "OmniVoice 支持的语言列表"},
	{Path: "/omnivoice/health", Handler: crystal.OmniVoiceHandler(OmniVoiceTTS.HealthHandler), Method: "GET", Description: "OmniVoice 引擎健康检查"},

	// ==================== LTPX 工具包调度 ====================
	// 同时作为进程内 Broker 的 HTTP 调试入口
	{Path: "/ltpx/tools", Handler: crystal.LTPRemoteToolsHandler, Method: "GET", Description: "拉取工具链（动态扫描包 AtoA 能力）"},
	{Path: "/ltpx/call", Handler: crystal.LTPRemoteCallHandler, Method: "POST", Description: "调用工具（异步受理，立即返回 request_id）"},
	{Path: "/ltpx/poll", Handler: crystal.LTPRemotePendingHandler, Method: "GET", Description: "列出待定工具调用（调试端点，已不承担轮询取结果）"},
	{Path: "/ltpx/result", Handler: crystal.LTPRemoteResultHandler, Method: "POST", Description: "前端包执行完毕后回执结果"},
	{Path: "/ltpx/event", Handler: crystal.LTPRemoteEventHandler, Method: "POST", Description: "事件推送（xx事件发生前负载经 LTP9 插件处理）"},
	{Path: "/mini-ltp-agent.js", Handler: crystal.MiniLTPAgentHandler, Method: "GET", Description: "通用页面操作智能体脚本（Mini-LTP）"},
	{Path: "/shared-input.js", Handler: crystal.SharedInputHandler, Method: "GET", Description: "统一键鼠操作共享模块"},
	{Path: "/self-ltp-agent.js", Handler: crystal.SelfLTPAgentHandler, Method: "GET", Description: "自主页面操作智能体脚本（Self-LTP）"},

	// ==================== WebView 深读代理（后端驱动的顶层 WebView 会话） ====================
	{Path: "/webview/open", Handler: crystal.WebViewOpenHandler, Method: "POST", Description: "创建 WebView 深读会话窗口"},
	{Path: "/webview/navigate", Handler: crystal.WebViewNavigateHandler, Method: "POST", Description: "深读会话导航到指定 URL 并等待就绪"},
	{Path: "/webview/eval", Handler: crystal.WebViewEvalHandler, Method: "POST", Description: "深读会话页面上下文执行脚本并取回 JSON 结果"},
	{Path: "/webview/ready", Handler: crystal.WebViewReadyHandler, Method: "POST", Description: "等待深读会话页面就绪并返回状态"},
	{Path: "/webview/screenshot", Handler: crystal.WebViewScreenshotHandler, Method: "POST", Description: "深读会话窗口区域截图（dataURL 返回）"},
	{Path: "/webview/close", Handler: crystal.WebViewCloseHandler, Method: "POST", Description: "关闭深读会话窗口"},
	{Path: "/webview/deepread", Handler: crystal.WebViewDeepReadHandler, Method: "POST", Description: "深读编排：导航→就绪→元素提取（可滚动）→截图→模型摘要"},
}
