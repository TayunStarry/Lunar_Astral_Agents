package server

import (
	"LunarCore/server/handlers"
	"LunarCore/server/handlers/image"
	storage "storage/server"
)

// SystemEndpoints 存储所有系统端点配置
var SystemEndpoints = []SystemEndpoint{
	// 文件读写相关接口
	{Path: "/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除操作"},
	{Path: "/file_list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表查询"},
	{Path: "/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载操作"},
	{Path: "/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档处理"},
	{Path: "/save", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存操作"},
	{Path: "/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取操作"},
	// 数据库相关接口
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据库管理"},
	// 图片生成相关接口
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成服务"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成结果"},
	// 视频处理相关接口
	{Path: "/extract/keyframes", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频切片提取"},
	// 智能体相关接口
	{Path: "/v1/models", Handler: handlers.AgentModelsHandler, Method: "GET", Description: "模型列表查询"},
	{Path: "/v1/", Handler: handlers.AgentHandler, Method: "POST", Description: "模型交互接口"},
	// 代理请求接口
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问服务"},
	// 消息队列相关接口
	{Path: "/write/message", Handler: handlers.MessageBatchHandler, Method: "POST", Description: "消息写入队列"},
	{Path: "/write/videourl", Handler: handlers.VideoUrlBatchHandler, Method: "POST", Description: "视频URL写入"},
	// TTS语音服务相关接口
	{Path: "/audio/generate", Handler: handlers.TTSProxyHandler, Method: "POST", Description: "TTS代理服务"},
	{Path: "/qwen_tts/models", Handler: handlers.TTSQwen3ProxyHandler, Method: "GET", Description: "TTS模型检测"},
	{Path: "/qwen_tts/", Handler: handlers.TTSQwen3ProxyHandler, Method: "POST", Description: "TTS语音服务"},
}
