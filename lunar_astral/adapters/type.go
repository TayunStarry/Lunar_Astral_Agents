package adapters

import "github.com/dop251/goja"

// IPInfo 存储IP地址信息
type IPInfo struct {
	Region string `json:"region"`
	City   string `json:"city"`
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
