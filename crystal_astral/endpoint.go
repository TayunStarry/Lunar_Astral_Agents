package main

import (
	"net/http"
	"screenshot"
	storage "storage/server"
)

type SystemEndpoint struct {
	Path        string
	Handler     http.HandlerFunc
	Method      string
	Description string
}

var SystemEndpoints = []SystemEndpoint{
	{Path: "/load/application", Handler: loadApplicationHandler, Method: "POST", Description: "加载应用"},
	{Path: "/background", Handler: storage.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},
	{Path: "/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file_list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/save", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取"},
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据管理"},
	{Path: "/capture", Handler: screenshot.HandleScreenshot, Method: "POST", Description: "通用截图"},
	{Path: "/capture/display/", Handler: screenshot.HandleScreenshotDisplay, Method: "GET", Description: "屏幕截图"},
	{Path: "/capture/region", Handler: screenshot.HandleScreenshotRegion, Method: "POST", Description: "区域截图"},
	{Path: "/capture/displays", Handler: screenshot.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: screenshot.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	{Path: "/api/proxy/models", Handler: modelsProxyHandler, Method: "POST", Description: "模型查询代理"},
	{Path: "/api/proxy/chat", Handler: chatProxyHandler, Method: "POST", Description: "对话代理"},
}
