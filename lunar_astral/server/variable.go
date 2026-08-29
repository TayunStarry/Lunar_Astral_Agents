package server

import (
	"LunarAstral/model"
	"LunarAstral/model/llama"
	"LunarAstral/server/handlers"
	file "LunarSubsystem/FileManager/server"
	"LunarSubsystem/GeneralConfig"
	image "LunarSubsystem/ImageProcessor/server"
	tts "LunarSubsystem/Qwen3-TTS/module"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// httpMux 是HTTP服务器的ServeMux实例
var httpMux *http.ServeMux

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{
	fmt.Sprintf("http://localhost:%d", *GeneralConfig.BasicPort),
	fmt.Sprintf("http://127.0.0.1:%d", *GeneralConfig.BasicPort),
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
	// ==== 应用与资源 ====
	{Path: "/background", Handler: file.RandomBackgroundHandler, Method: "GET", Description: "随机背景图片"},
	// ==== 文件操作 ====
	{Path: "/file/write", Handler: file.SaveHandler, Method: "POST", Description: "文件保存操作"},
	{Path: "/file/read/", Handler: file.ReadHandler, Method: "GET", Description: "文件读取操作"},
	// ==== 记忆库 ====
	{Path: "/memory/", Handler: file.MemoryHandler, Method: "ANY", Description: "记忆库（实例初始化/集合管理/消息增删查/文档列表/重建）"},
	// ==== 图片生成 ====
	{Path: "/generate", Handler: image.GenerateHandler, Method: "POST", Description: "图片生成服务"},
	{Path: "/generate/wait", Handler: image.GenerateWaitHandler, Method: "GET", Description: "等待生成结果"},
	// ==== 截图与图像处理 ====
	{Path: "/capture", Handler: image.HandleCapture, Method: "ANY", Description: "统一截图（auto/window/fullscreen/display/region）"},
	{Path: "/capture/displays", Handler: image.HandleGetDisplays, Method: "GET", Description: "屏幕列表"},
	{Path: "/resize", Handler: image.HandleResizeImage, Method: "POST", Description: "图片缩放"},
	// ==== 智能体相关接口 - 代理到 llama.cpp 服务器（支持所有 HTTP 方法） ====
	{Path: "/v1/", Handler: llama.ProxyHandler, Method: "ANY", Description: "llama.cpp 代理接口"},
	// ==== 代理请求接口 ====
	{Path: "/proxy", Handler: handlers.ProxyHandler, Method: "POST", Description: "代理访问服务"},
	// ==== 消息队列相关接口 ====
	{Path: "/write/message", Handler: handlers.MessageBatchHandler, Method: "POST", Description: "消息写入队列"},
	{Path: "/write/videourl", Handler: handlers.VideoUrlBatchHandler, Method: "POST", Description: "视频URL写入"},
	// ==== 引擎消息总线（格式与 /write/message 同构，供引擎/工作室系统消息分发） ====
	{Path: "/write/engine", Handler: handlers.EngineMessageHandler, Method: "POST", Description: "引擎系统消息（动画列表/遥测等）"},
	// ==== TTS语音服务相关接口 ====
	{Path: "/tts", Handler: tts.TTSHandler, Method: "POST", Description: "TTS语音合成服务"},
	// ==== LTPX 远程（琉璃）协调接口 ====
	{Path: "/ltpx/register", Handler: handlers.LTPXRemoteRegisterHandler, Method: "POST", Description: "琉璃启动时提交联络URL"},
	// ==== 智能体控制接口 ====
	{Path: "/write/agent_position", Handler: handlers.AgentPositionHandler, Method: "POST", Description: "更新智能体3D位置"},
	{Path: "/write/agent_event", Handler: handlers.AgentEventHandler, Method: "POST", Description: "推送引擎事件到AI上下文"},
}
