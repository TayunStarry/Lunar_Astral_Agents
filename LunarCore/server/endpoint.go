package server

import (
	"LunarCore/server/handlers"
	"LunarCore/server/handlers/file"
	"LunarCore/server/handlers/file/image"
	"LunarCore/server/handlers/file/memory"
)

// SystemEndpoints 存储所有系统端点配置
var SystemEndpoints = []SystemEndpoint{
	// 文件读写相关接口
	{Path: "/delete/", Handler: file.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file_list/", Handler: file.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/download/", Handler: file.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/archive", Handler: file.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/save", Handler: file.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/read/", Handler: file.ReadHandler, Method: "GET", Description: "文件读取"},
	// 知识库相关接口
	{Path: "/knowledge/query", Handler: memory.KnowledgeQueryHandler, Method: "POST", Description: "知识查询"},
	{Path: "/knowledge/write", Handler: memory.KnowledgeWriteHandler, Method: "POST", Description: "知识写入"},
	{Path: "/knowledge/flush", Handler: memory.KnowledgeFlushHandler, Method: "POST", Description: "知识刷新"},
	{Path: "/knowledge/delete", Handler: memory.KnowledgeDeleteHandler, Method: "POST", Description: "知识删除"},
	{Path: "/knowledge/list", Handler: memory.KnowledgeListHandler, Method: "GET", Description: "知识列表"},
	// 数据库相关接口
	{Path: "/database/", Handler: memory.DatabaseHandler, Method: "POST", Description: "数据管理"},
	// 图片生成相关接口
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成"},
	// 屏幕截图相关接口
	{Path: "/capture", Handler: image.HandleCapture, Method: "POST", Description: "通用截图"},
	{Path: "/capture/display/", Handler: image.HandleCaptureDisplay, Method: "GET", Description: "屏幕截图"},
	{Path: "/capture/region", Handler: image.HandleCaptureRegion, Method: "POST", Description: "区域截图"},
	{Path: "/capture/displays", Handler: image.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	// 图片处理相关接口
	{Path: "/resize", Handler: image.HandleResizeImage, Method: "POST", Description: "图片处理"},
	// 视频处理相关接口
	{Path: "/extract/keyframes", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频切片"},
	{Path: "/extract/firstframe", Handler: image.ExtractFirstFrameHandler, Method: "POST", Description: "视频首帧"},
	// 清理相关接口
	{Path: "/cleanup/images", Handler: image.CleanupUnreferencedImagesHandler, Method: "POST", Description: "清理图片"},
	// 智能体相关接口
	{Path: "/v1/models", Handler: handlers.AgentModelsHandler, Method: "GET", Description: "模型列表"},
	{Path: "/v1/", Handler: handlers.AgentHandler, Method: "POST", Description: "模型交互"},
	// 代理请求接口
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问"},
	// 消息队列相关接口
	{Path: "/write/message", Handler: handlers.MessageBatchHandler, Method: "POST", Description: "批量消息写入"},
	{Path: "/write/videourl", Handler: handlers.VideoUrlBatchHandler, Method: "POST", Description: "批量视频URL写入"},
	// WebView 控制接口
	{Path: "/webview/control", Handler: handlers.WebViewControlHandler, Method: "POST", Description: "页面控制"},
}
