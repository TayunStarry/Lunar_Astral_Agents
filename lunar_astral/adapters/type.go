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

// LTPXRemoteToolDef 琉璃工具链中的单个工具定义
// 琉璃对外统一暴露「智能体式」工具：接受字符串指令，返回文本结果（含操作结果与推荐后续操作）
type LTPXRemoteToolDef struct {
	Name        string `json:"name"`        // 工具名（注入月华 LTPdefinition）
	Description string `json:"description"` // 工具能力描述（供 LLM 决策）
	AppID       string `json:"app_id"`      // 关联的琉璃应用标识（如 lunar.means.file.explorer）
	Parameters  any    `json:"parameters"`  // JSON Schema 参数定义
}

// LTPXRemoteRegisterRequest 琉璃启动时提交联络 URL 的请求体
type LTPXRemoteRegisterRequest struct {
	URL string `json:"url"` // 琉璃自身可访问的地址（如 http://localhost:XXXXX）
}

// LTPXRemoteStatusResult 月华同步琉璃工具链的返回结果
type LTPXRemoteStatusResult struct {
	Online bool                `json:"online"` // 琉璃是否在线
	URL    string              `json:"url"`    // 当前记录的琉璃 URL（空表示未注册）
	Tools  []LTPXRemoteToolDef `json:"tools"`  // 最新工具链
}

// LTPXRemoteCallRequest 月华请求调用琉璃工具的请求体
type LTPXRemoteCallRequest struct {
	Tool      string         `json:"tool"`   // 工具名
	Arguments map[string]any `json:"arguments"` // 工具参数
}

// LTPXRemoteCallResponse 琉璃执行工具后的返回体
type LTPXRemoteCallResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text"` // 操作结果文本（含推荐后续操作）
	Error   string `json:"error,omitempty"`
}

// PushContextData 推送上下文数据
type PushContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Audio   string `json:"audio,omitempty"`
}

// PushImageData 推送图片数据
type PushImageData struct {
	Type    string   `json:"type"`              // 图片类型
	Sticker bool     `json:"sticker,omitempty"` // 是否作为表情包发送
	Images  []string `json:"images"`            // 图片URL列表
}

// AgentPositionData 智能体3D位置数据
type AgentPositionData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
