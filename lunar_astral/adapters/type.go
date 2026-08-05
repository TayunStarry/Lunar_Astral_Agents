package adapters

import "github.com/dop251/goja"

// IPInfo 存储IP地址信息（对应 ip-api.com 响应结构）
type IPInfo struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
}

// Runtime 存储JavaScript运行时实例，用于调用适配器函数
type Runtime struct {
	runtime *goja.Runtime
}

// TextContent 文本内容
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ImageContent 图片内容
type ImageContent struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

// PostMessageRole 消息角色类型
type PostMessageRole string

const (
	RoleUser      PostMessageRole = "user"
	RoleAssistant PostMessageRole = "assistant"
	RoleSystem    PostMessageRole = "system"
	RoleTool      PostMessageRole = "tool"
)

// PostMessage 消息结构体
type PostMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // 可以是string或[]MessageContent
}

// LTPXPackageInfo LTPX 工具包配置结构
type LTPXPackageInfo struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	URL         string           `json:"url"`
	Tools       []map[string]any `json:"tools"`
}

// LTPXToolInfo 已加载工具的内部状态
type LTPXToolInfo struct {
	Name       string `json:"name"`
	Definition string `json:"definition"` // 工具定义 JSON 字符串
	JS         string `json:"js"`         // tool.js 源码
}

// LTPXStatus 工具状态查询结果
type LTPXStatus struct {
	Loaded         []string        `json:"loaded"`
	PendingLoads   []*LTPXToolInfo `json:"pendingLoads"`
	PendingUnloads []string        `json:"pendingUnloads"`
}

// PushContextData 推送上下文数据
type PushContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Audio   string `json:"audio,omitempty"`
}

// PushImageData 推送图片数据
type PushImageData struct {
	Type   string   `json:"type"`
	Images []string `json:"images"`
}

// AgentPositionData 智能体3D位置数据
type AgentPositionData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
