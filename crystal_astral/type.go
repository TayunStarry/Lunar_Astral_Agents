package main

import (
	"net/http"
	"net/http/httputil"

	"github.com/gorilla/websocket"
)

// LoadApplicationRequest 加载应用请求结构体
type LoadApplicationRequest struct {
	Path string `json:"path"` // 应用路径
}

// LoadApplicationResponse 加载应用响应结构体
type LoadApplicationResponse struct {
	Success bool   `json:"success"`           // 是否成功加载应用
	Message string `json:"message,omitempty"` // 加载应用的消息提示
}

// PackageInfo 包配置信息
type PackageInfo struct {
	ID          string   `json:"id"`                     // 包ID，唯一标识一个应用
	Icon        string   `json:"icon,omitempty"`         // 包图标路径
	Title       string   `json:"title"`                  // 包标题，显示在应用列表中
	Description string   `json:"description"`            // 包描述，显示在应用列表中，描述应用的功能
	URL         string   `json:"url,omitempty"`          // 包的URL，用于下载应用
	Path        string   `json:"path,omitempty"`         // 包的本地路径，用于加载应用
	Tags        []string `json:"tags,omitempty"`         // 包的标签，用于分类应用
	PackageName string   `json:"package_name,omitempty"` // 包的名称，用于显示在应用列表中，描述应用的功能或来源
}

// proxyAwareHandler 代理感知处理程序
// 用于在处理请求时根据路径判断是否需要通过代理转发
type proxyAwareHandler struct {
	fs          http.Handler           // 文件系统处理程序，用于处理静态文件请求
	proxy       *httputil.ReverseProxy // 反向代理，用于将请求转发到其他服务器
	shouldProxy func(string) bool      // 判断是否需要通过代理转发的函数
}

// LunarCheckResponse 月华服务检测响应结构体
type LunarCheckResponse struct {
	Available bool `json:"available"` // 是否可用
}

// LunarStartResponse 月华服务启动响应结构体
type LunarStartResponse struct {
	Success bool   `json:"success"`           // 是否成功启动月华服务
	Message string `json:"message,omitempty"` // 启动月华服务的消息提示
}

// SystemEndpoint 系统端点
type SystemEndpoint struct {
	Path        string           // Path 端点路径
	Handler     http.HandlerFunc // Handler 处理函数
	Method      string           // Method 请求方法
	Description string           // Description 描述端点的功能
}

// ChatProxyRequest 对话代理请求结构体
type ChatProxyRequest struct {
	BaseURL  string                   `json:"base_url"`
	APIKey   string                   `json:"api_key"`
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// ChatProxyResponse 对话代理响应结构体
type ChatProxyResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ModelProxyRequest 模型代理请求结构体
type ModelProxyRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// ModelProxyResponse 模型代理响应结构体
type ModelProxyResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ==== WebSocket 工作室消息中枢 ====

// ==== LTPX 远程（月华调用）协议类型 ====

// LTPXRemoteToolDef 对外暴露的「智能体式」工具定义（新增版 LTPX）
// 统一接收字符串指令，返回文本结果（含操作结果与推荐后续操作）
type LTPXRemoteToolDef struct {
	Name        string `json:"name"`        // 工具名
	Description string `json:"description"` // 工具能力描述（供 LLM 决策）
	AppID       string `json:"app_id"`      // 关联的应用标识（如 file.manager）
	Parameters  any    `json:"parameters"`  // JSON Schema 参数定义
}

// LTPXRemoteCallRequest 月华请求调用琉璃工具的请求体
type LTPXRemoteCallRequest struct {
	Tool      string         `json:"tool"`      // 工具名
	Arguments map[string]any `json:"arguments"` // 工具参数
}

// LTPXRemoteCallResponse 琉璃执行工具后的返回体
type LTPXRemoteCallResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text"` // 操作结果文本（含推荐后续操作）
	Error   string `json:"error,omitempty"`
}

// LTPXResultRequest 前端包执行完毕后向琉璃回执结果的请求体
type LTPXResultRequest struct {
	RequestID string `json:"request_id"`        // 调用请求 ID（ltpx_call 广播时下发）
	Success   bool   `json:"success"`           // 是否执行成功
	Text      string `json:"text,omitempty"`    // 操作结果文本
	Error     string `json:"error,omitempty"`   // 错误信息
	KeepOpen  bool   `json:"keep_open,omitempty"` // 包是否要求执行后保持页面展示（如文件管理器）
}

// LunarRegisterResponse 月华返回的注册响应
type LunarRegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// StartupVoice 启动时语音决策（由前端读取以自动播放对应语音）
type StartupVoice struct {
	Voice string `json:"voice"` // "sent"（工具包已发送）/ "failed"（无法交给月华）
	Lunar bool   `json:"lunar"` // 月华是否在线
	Seq   int64  `json:"seq"`   // 决策序号（每次琉璃进程启动递增），前端据此去重，避免 WS 重连导致重复播放
}

// StudioClient 工作室 WebSocket 客户端连接
type StudioClient struct {
	Conn *websocket.Conn // WebSocket 连接
	Send chan []byte     // 发送消息的缓冲通道
}

// StudioHub 工作室 WebSocket 消息中枢
// 职责：接受所有客户端连接（/ws），将任意客户端发来的 JSON 消息广播给所有已连接客户端
// 设计原则：不解析消息内容，纯粹转发 JSON 字节流（无差别广播，客户端自行过滤）
type StudioHub struct {
	Clients    map[*StudioClient]bool // 已注册的客户端集合
	Broadcast  chan []byte            // 广播消息通道
	Register   chan *StudioClient     // 客户端注册通道
	Unregister chan *StudioClient     // 客户端注销通道
}