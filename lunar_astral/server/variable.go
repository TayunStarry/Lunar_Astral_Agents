package server

import (
	"config"
	"fmt"
	image "image/server"
	"lunar_astral/model"
	"lunar_astral/model/llama"
	"lunar_astral/model/tts"
	"lunar_astral/server/handlers"
	"net/http"
	storage "storage/server"
	"sync"

	"github.com/gorilla/websocket"
)

// httpMux 是HTTP服务器的ServeMux实例
var httpMux *http.ServeMux

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{
	fmt.Sprintf("http://localhost:%d", *config.BasicPort),
	fmt.Sprintf("http://127.0.0.1:%d", *config.BasicPort),
}

// 请求映射，键为请求ID，值为请求上下文
var requests = make(map[string]*model.RequestContext)

// 互斥锁，用于保护请求映射的并发访问
var serverMutex sync.RWMutex

// WebSocket 升级器，用于将HTTP连接升级为WebSocket连接
var upgrader = websocket.Upgrader{
	// 读取缓冲区大小，用于接收客户端发送的消息
	ReadBufferSize: 1024,
	// 写入缓冲区大小，用于发送消息给客户端
	WriteBufferSize: 1024,
	// 检查请求来源是否在允许列表中
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocket 客户端映射，键为客户端连接，值为true
var wsClients = make(map[*WSClient]bool)

// WebSocket 客户端互斥锁，用于保护客户端映射的并发访问
var wsMutex sync.RWMutex

// WebSocket 广播通道，用于发送消息给所有客户端
var wsBroadcaster = make(chan WSMessage, 256)

// SystemEndpoints 存储所有系统端点配置
var SystemEndpoints = []SystemEndpoint{
	// 文件读写相关接口
	{Path: "/background", Handler: storage.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},
	{Path: "/file/delete/", Handler: storage.DeleteHandler, Method: "DELETE", Description: "文件删除操作"},
	{Path: "/file/list/", Handler: storage.FileListHandler, Method: "POST", Description: "文件列表查询"},
	{Path: "/file/download/", Handler: storage.DownloadHandler, Method: "GET", Description: "文件下载操作"},
	{Path: "/file/archive", Handler: storage.ArchiveHandler, Method: "POST", Description: "文件归档处理"},
	{Path: "/file/package/install", Handler: storage.InstallPackageHandler, Method: "POST", Description: "安装扩展包"},
	{Path: "/file/package/export", Handler: storage.ExportPackageHandler, Method: "POST", Description: "导出扩展包"},
	{Path: "/file/package/delete", Handler: storage.DeletePackageHandler, Method: "POST", Description: "删除扩展包"},
	{Path: "/file/write", Handler: storage.SaveHandler, Method: "POST", Description: "文件保存操作"},
	{Path: "/file/read/", Handler: storage.ReadHandler, Method: "GET", Description: "文件读取操作"},
	// 数据库相关接口
	{Path: "/database/", Handler: storage.DatabaseHandler, Method: "POST", Description: "数据库管理"},
	// 向量数据库相关接口
	{Path: "/chromem/init", Handler: storage.ChromemInitHandler, Method: "POST", Description: "向量数据库初始化"},
	{Path: "/chromem/messages", Handler: storage.ChromemMessagesHandler, Method: "POST", Description: "向量数据库消息管理"},
	{Path: "/chromem/rebuild", Handler: storage.ChromemRebuildHandler, Method: "POST", Description: "重建向量数据库文档索引"},
	{Path: "/chromem/stats", Handler: storage.ChromemStatsHandler, Method: "GET", Description: "向量数据库统计信息"},
	{Path: "/chromem/documents", Handler: storage.ChromemDocumentsHandler, Method: "GET", Description: "向量数据库文档列表"},
	// 图片生成相关接口
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成服务"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成结果"},
	// 视频处理相关接口
	{Path: "/extract/keyframes", Handler: image.ExtractKeyFramesHandler, Method: "POST", Description: "视频切片提取"},
	// 智能体相关接口 - 代理到 llama.cpp 服务器（支持所有 HTTP 方法）
	{Path: "/v1/", Handler: llama.ProxyHandler, Method: "ANY", Description: "llama.cpp 代理接口"},
	// 代理请求接口
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问服务"},
	// 消息队列相关接口
	{Path: "/write/message", Handler: handlers.MessageBatchHandler, Method: "POST", Description: "消息写入队列"},
	{Path: "/write/videourl", Handler: handlers.VideoUrlBatchHandler, Method: "POST", Description: "视频URL写入"},
	// TTS语音服务相关接口
	{Path: "/tts", Handler: tts.TTSHandlerWrapper, Method: "POST", Description: "TTS语音合成服务"},
	{Path: "/tts/stream", Handler: tts.TTSStreamHandlerWrapper, Method: "GET", Description: "TTS流式合成服务"},
}
