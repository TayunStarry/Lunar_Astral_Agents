package main

import (
	"net/http"
	"screenshot"
	storage "storage/server"
)

// SystemEndpoint 系统端点
type SystemEndpoint struct {
	// Path 端点路径
	Path string
	// Handler 处理函数
	Handler http.HandlerFunc
	// Method 请求方法
	Method string
	// Description 描述端点的功能
	Description string
}

// SystemEndpoints 系统端点列表
var SystemEndpoints = []SystemEndpoint{
	{Path: "/load/application", Handler: loadApplicationHandler, Method: "POST", Description: "加载应用"},
	{Path: "/background", Handler: storage.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},
	{Path: "/file/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file/list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/file/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/file/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/file/write", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/file/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取"},
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据管理"},
	{Path: "/capture", Handler: screenshot.HandleScreenshot, Method: "POST", Description: "通用截图"},
	{Path: "/capture/display/", Handler: screenshot.HandleScreenshotDisplay, Method: "GET", Description: "屏幕截图"},
	{Path: "/capture/region", Handler: screenshot.HandleScreenshotRegion, Method: "POST", Description: "区域截图"},
	{Path: "/capture/displays", Handler: screenshot.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: screenshot.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	{Path: "/chromem/init", Handler: storage.ChromemInitHandler, Method: "POST", Description: "向量数据库初始化"},
	{Path: "/chromem/messages", Handler: storage.ChromemMessagesHandler, Method: "POST", Description: "向量数据库消息管理"},
	{Path: "/chromem/rebuild", Handler: storage.ChromemRebuildHandler, Method: "POST", Description: "重建向量数据库文档索引"},
	{Path: "/chromem/stats", Handler: storage.ChromemStatsHandler, Method: "GET", Description: "向量数据库统计信息"},
	{Path: "/chromem/documents", Handler: storage.ChromemDocumentsHandler, Method: "GET", Description: "向量数据库文档列表"},
	{Path: "/api/proxy/models", Handler: modelsProxyHandler, Method: "POST", Description: "模型查询代理"},
	{Path: "/api/proxy/chat", Handler: chatProxyHandler, Method: "POST", Description: "对话代理"},
}
