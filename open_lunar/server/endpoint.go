package server

import (
	"Lunar-Astral-Agents/server/handlers"
	"net/http"
)

// SystemEndpoint 定义系统端点的结构
type SystemEndpoint struct {
	// HTTP 访问路径
	Path string `json:"path"`
	// HTTP 方法处理器
	Handler http.HandlerFunc `json:"handler"`
	// HTTP 方法类型
	Method string `json:"method"`
	// 处理器功能描述
	Description string `json:"description"`
}

// SystemEndpoints 存储所有系统端点配置
var SystemEndpoints = []SystemEndpoint{
	// 文件读写相关接口
	{Path: "/delete/", Handler: handlers.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file_list/", Handler: handlers.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/download/", Handler: handlers.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/archive", Handler: handlers.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/save", Handler: handlers.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/read/", Handler: handlers.ReadHandler, Method: "GET", Description: "文件读取"},
	// 智能体相关接口
	{Path: "/v1/models", Handler: handlers.AgentModelsHandler, Method: "GET", Description: "模型列表"},
	{Path: "/v1/chat/", Handler: handlers.AgentChatHandler, Method: "POST", Description: "模型对话"},
	{Path: "/v1/", Handler: handlers.AgentHandler, Method: "POST", Description: "模型交互"},
	// 图片生成相关接口
	{Path: "/generate", Handler: handlers.GenerateHandler, Method: "POST", Description: "图片生成"},
	{Path: "/generate/wait", Handler: handlers.GenerateWaitHandler, Method: "GET", Description: "等待生成"},
	// 知识库相关接口
	{Path: "/knowledge/query", Handler: handlers.KnowledgeQueryHandler, Method: "POST", Description: "知识查询"},
	{Path: "/knowledge/write", Handler: handlers.KnowledgeWriteHandler, Method: "POST", Description: "知识写入"},
	{Path: "/knowledge/flush", Handler: handlers.KnowledgeFlushHandler, Method: "POST", Description: "知识刷新"},
	{Path: "/knowledge/delete", Handler: handlers.KnowledgeDeleteHandler, Method: "POST", Description: "知识删除"},
	{Path: "/knowledge/list", Handler: handlers.KnowledgeListHandler, Method: "GET", Description: "知识列表"},
	// 屏幕截图相关接口
	{Path: "/capture", Handler: handlers.HandleCapture, Method: "POST", Description: "通用截图"},
	{Path: "/capture/display/", Handler: handlers.HandleCaptureDisplay, Method: "GET", Description: "屏幕截图"},
	{Path: "/capture/region", Handler: handlers.HandleCaptureRegion, Method: "POST", Description: "区域截图"},
	{Path: "/capture/displays", Handler: handlers.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	// 图片处理相关接口
	{Path: "/resize", Handler: handlers.HandleResizeImage, Method: "POST", Description: "图片处理"},
	// 视频处理相关接口
	{Path: "/extract/keyframes", Handler: handlers.ExtractKeyFramesHandler, Method: "POST", Description: "视频切片"},
	{Path: "/extract/firstframe", Handler: handlers.ExtractFirstFrameHandler, Method: "POST", Description: "视频首帧"},
	// 数据库相关接口
	{Path: "/database/", Handler: handlers.DatabaseHandler, Method: "POST", Description: "数据管理"},
	// 清理相关接口
	{Path: "/cleanup/images", Handler: handlers.CleanupUnreferencedImagesHandler, Method: "POST", Description: "清理图片"},
	// 代理请求接口
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问"},
	// WebView 控制接口
	{Path: "/webview/control", Handler: handlers.WebViewControlHandler, Method: "POST", Description: "页面控制"},
}
