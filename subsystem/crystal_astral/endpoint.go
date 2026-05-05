package main

import (
	"net/http"
	storage "storage/server"
)

type SystemEndpoint struct {
	Path        string
	Handler     http.HandlerFunc
	Method      string
	Description string
}

var SystemEndpoints = []SystemEndpoint{
	{Path: "/background", Handler: serveRandomBackground, Method: "GET", Description: "随机背景图片"},
	{Path: "/load/application", Handler: loadApplicationHandler, Method: "POST", Description: "加载应用"},
	{Path: "/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除"},
	{Path: "/file_list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表"},
	{Path: "/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载"},
	{Path: "/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档"},
	{Path: "/save", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存"},
	{Path: "/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取"},
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据管理"},
}
