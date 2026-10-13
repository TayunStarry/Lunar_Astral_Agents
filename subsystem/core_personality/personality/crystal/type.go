package crystal

import (
	"time"
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
	ID                  string   `json:"id"`                             // 包ID，唯一标识一个应用
	Icon                string   `json:"icon,omitempty"`                 // 包图标路径
	Title               string   `json:"title"`                          // 包标题，显示在应用列表中
	Description         string   `json:"description"`                    // 包描述
	URL                 string   `json:"url,omitempty"`                  // 包的URL，用于下载应用
	Path                string   `json:"path,omitempty"`                 // 包的本地路径，用于加载应用
	Tags                []string `json:"tags,omitempty"`                 // 包的标签，用于分类应用
	PackageName         string   `json:"package_name,omitempty"`         // 包的名称
	BackgroundRetention bool     `json:"background_retention,omitempty"` // 后台保活
}

// ModuleCreateRequest 创建模块请求体（琉璃前端「创建模块」弹窗提交）
type ModuleCreateRequest struct {
	PackageName string   `json:"package_name"` // 包目录名（可选，缺省由标题/ID 推导）
	ID          string   `json:"id"`           // 包 ID（可选，缺省自动生成 module.<包名>）
	Title       string   `json:"title"`        // 包标题
	Description string   `json:"description"`  // 包描述
	Icon        string   `json:"icon"`         // 图标：空 / data:image/...;base64 / 相对路径或 URL
	URL         string   `json:"url"`          // 外部链接(http/https) 或 本地 HTML 文件/目录路径
	Path        string   `json:"path"`         // 本地程序路径（exe/ps1/bat/cmd/lnk）
	MiniLTP     bool     `json:"mini_ltp"`     // 是否启用 Mini-LTP
	ToolName    string   `json:"tool_name"`    // AtoA 工具名
	Tags        []string `json:"tags"`         // 附加标签
	ZipPath     string   `json:"-"`            // 内部：ZIP 上传解压前的临时文件路径
}

// ModuleCreateResponse 创建模块响应体
type ModuleCreateResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	PackageName string `json:"package_name,omitempty"`
	PackageID   string `json:"package_id,omitempty"`
}

// ModuleInspectField 提取出的项目元信息片段
type ModuleInspectField struct {
	Key  string `json:"key"`  // 字段名（title / README / filenames）
	Text string `json:"text"` // 片段内容（截断）
}

// ModuleInspectRequest 项目内容检查请求体：URL/路径（JSON）或 ZIP（multipart）
type ModuleInspectRequest struct {
	URL     string `json:"url"` // 外部链接或本地路径（可为空，配合 zip_file）
	ZipFile string `json:"-"`   // 内部：上传的 ZIP 临时路径
}

// ModuleInspectResponse 项目内容检查响应体
type ModuleInspectResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message,omitempty"`
	Name    string               `json:"name,omitempty"`   // 项目名（目录/文件名）
	Fields  []ModuleInspectField `json:"fields,omitempty"` // 提取到的元信息片段
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

// ==== LTPX 远程（工具包调度）协议类型 ====

// LTPXRemoteToolDef 对外暴露的「智能体式」工具定义
// 统一接收字符串指令，返回文本结果（含操作结果与推荐后续操作）
type LTPXRemoteToolDef struct {
	Name        string `json:"name"`        // 工具名
	Description string `json:"description"` // 工具能力描述（供 LLM 决策）
	Parameters  any    `json:"parameters"`  // JSON Schema 参数定义
}

// LTPXRemoteCallRequest 调用琉璃工具的请求体
type LTPXRemoteCallRequest struct {
	Tool      string         `json:"tool"`      // 工具名
	Arguments map[string]any `json:"arguments"` // 工具参数
}

// LTPXRemoteCallResponse 工具执行后的返回体
type LTPXRemoteCallResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text"` // 操作结果文本（含推荐后续操作）
	Error   string `json:"error,omitempty"`
	// Pending 异步受理标记：受理后立即返回 pending=true + request_id，
	// 结果由调用方经结果通道取回（覆盖长时间运行的联网搜索等任务）
	Pending   bool   `json:"pending,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ltpPendingCall 待定调用条目：结果通道 + 元信息（供结果取回与超时清理）
type ltpPendingCall struct {
	done      chan LTPXRemoteCallResponse // 结果通道（容量 1）
	toolName  string                      // 目标工具名（日志用）
	startedAt time.Time                   // 受理时间（超过最长执行时限时清理）
}

// LTPXResultRequest 前端包执行完毕后回执结果的请求体
type LTPXResultRequest struct {
	RequestID string `json:"request_id"`          // 调用请求 ID（ltpx_call 广播时下发）
	Success   bool   `json:"success"`             // 是否执行成功
	Text      string `json:"text,omitempty"`      // 操作结果文本
	Error     string `json:"error,omitempty"`     // 错误信息
	KeepOpen  bool   `json:"keep_open,omitempty"` // 包是否要求执行后保持页面展示
}

// LTPXRemoteEventRequest 事件推送请求体（POST /ltpx/event）
// 调用方在每个「xx事件发生前」触发点把原始负载推送过来，经 LTP9 引擎派发到订阅插件。
type LTPXRemoteEventRequest struct {
	Topic   string `json:"topic,omitempty"` // 事件主题（如 message_received_before）
	Event   string `json:"event,omitempty"` // 事件中文名（备用，优先用 topic）
	Payload any    `json:"payload,omitempty"`
	Time    int64  `json:"time,omitempty"` // 事件发生时间戳（Unix 秒）
}
