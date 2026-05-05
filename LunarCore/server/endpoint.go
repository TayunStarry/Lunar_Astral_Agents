package server

import (
	"LunarCore/server/handlers"
	"LunarCore/server/handlers/image"
	storage "storage/server"
)

// SystemEndpoints 存储所有系统端点配置
var SystemEndpoints = []SystemEndpoint{
	// 文件读写相关接口
	{Path: "/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file_list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/save", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取"},
	// 数据库相关接口
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据管理"},
	// 图片生成相关接口
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成"},
	// 视频处理相关接口
	{Path: "/extract/keyframes", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频切片"},
	// 智能体相关接口
	{Path: "/v1/models", Handler: handlers.AgentModelsHandler, Method: "GET", Description: "模型列表"},
	{Path: "/v1/", Handler: handlers.AgentHandler, Method: "POST", Description: "模型交互"},
	// 代理请求接口
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问"},
	// 消息队列相关接口
	{Path: "/write/message", Handler: handlers.MessageBatchHandler, Method: "POST", Description: "批量消息写入"},
	{Path: "/write/videourl", Handler: handlers.VideoUrlBatchHandler, Method: "POST", Description: "批量视频URL写入"},
	// TTS语音服务相关接口
	{Path: "/audio/generate", Handler: handlers.TTSProxyHandler, Method: "POST", Description: "TTS语音服务代理"},
	{Path: "/qwen_tts/models", Handler: handlers.TTSQwen3ProxyHandler, Method: "GET", Description: "Qwen3模型检测"},
	{Path: "/qwen_tts/", Handler: handlers.TTSQwen3ProxyHandler, Method: "POST", Description: "Qwen3语音服务代理"},
}
