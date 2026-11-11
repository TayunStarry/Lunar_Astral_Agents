package server

import (
	"encoding/json"
	"net/http"
)

// StaticRoots 前端静态资源根（各前端 + 共享依赖）
type StaticRoots struct {
	// Lunar 月华消息终端（挂载于 /）
	Lunar http.FileSystem
	// Mobile 月华移动端（挂载于 /mobile/）
	Mobile http.FileSystem
	// Crystal 琉璃桌面（挂载于 /desktop/）
	Crystal http.FileSystem
	// Library 资源库（挂载于 /library/）
	Library http.FileSystem
	// Reliance 前端共享依赖（挂载于 /reliance/）
	Reliance http.FileSystem
	// Shared 桌面端与移动端共用 ES Module 共享层（挂载于 /shared/）
	Shared http.FileSystem
}

// Ready 月华消息终端与琉璃桌面的静态资源是否均已就绪
func (r StaticRoots) Ready() bool {
	return r.Lunar != nil && r.Crystal != nil
}

// WindowFocusRequest /window/focus 请求体
type WindowFocusRequest struct {
	// ID 目标桌面窗口标识（lunar / crystal）
	ID string `json:"id"`
}

// WindowFocusResponse /window/focus 响应体
type WindowFocusResponse struct {
	// Success 是否成功
	Success bool `json:"success"`
	// Message 结果说明
	Message string `json:"message"`
}

// MediaPlatformHostsResponse 共享媒体平台域名表响应（前端抽取 webpage 信封时使用）
type MediaPlatformHostsResponse struct {
	// VideoPlatformHosts 共享视频平台域名（与 engine/mediaobject.VideoPlatformHosts 同源）
	VideoPlatformHosts []string `json:"video_platform_hosts"`
}

// ProxyRequest 代理请求结构（/proxy 入参）
type ProxyRequest struct {
	URL         string      `json:"url"`
	RequestInit RequestInit `json:"requestInit"`
}

// RequestInit 请求初始化参数
type RequestInit struct {
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
	Redirect    string            `json:"redirect,omitempty"`
	Credentials string            `json:"credentials,omitempty"`
}

// ProxyResponse 代理响应结构
type ProxyResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}

// SystemEndpoint 单条系统端点定义（统一路由注册表的元素）。
// Handler 与 Path 由 server 装配层消费，Method/Description 供 /api-docs 与自检使用。
type SystemEndpoint struct {
	// Path 端点路径（ServeMux 模式，末尾带 / 表示子树匹配）
	Path string `json:"path"`
	// Handler 处理函数
	Handler http.HandlerFunc `json:"handler"`
	// Method 允许的方法（"GET"/"POST"/"ANY" 等，仅文档与自检用途）
	Method string `json:"method"`
	// Description 端点说明（供自述文档展示）
	Description string `json:"description"`
}

// AgentChatMessage /agent 非流式补全的助手消息
type AgentChatMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

// AgentChatChoice /agent 非流式补全的单条选项
type AgentChatChoice struct {
	Index        int              `json:"index"`
	Message      AgentChatMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

// AgentChatResponse /agent 端点响应（非流式补全或错误）
type AgentChatResponse struct {
	Object  string            `json:"object"`
	Model   string            `json:"model,omitempty"`
	Choices []AgentChatChoice `json:"choices,omitempty"`
	Error   *AgentChatError   `json:"error,omitempty"`
}

// AgentChatError /agent 端点错误主体
type AgentChatError struct {
	Message string `json:"message"`
}

// AgentRequest 外部推理排队请求（server 只声明结构，入队实现由装配层注入）
type AgentRequest struct {
	// Messages 完整消息数组（原样透传给模型）
	Messages json.RawMessage
	// Params 其余请求参数（temperature / top_p / max_tokens 等）
	Params map[string]json.RawMessage
	// Reply 结果回执（有且仅有一次写入）
	Reply chan AgentResult
}

// AgentResult 外部推理结果
type AgentResult struct {
	Content      string
	Reasoning    string
	ToolCalls    json.RawMessage
	FinishReason string
	Err          error
}

// LibraryCreationRequest 创作提交请求体（WS 命令 creation.submit）：资源句柄 + 自然语言描述。
type LibraryCreationRequest struct {
	// Mode assigned（默认，需要 description）或 random（随机目标池自主选题）
	Mode string `json:"mode"`
	// Description 自然语言描述
	Description string `json:"description"`
	// Handle 单个目标资源句柄
	Handle string `json:"handle"`
	// Handles 多个目标资源句柄
	Handles []string `json:"handles"`
}

// LibraryCreationTask 创作任务对外状态（提交、查询、推送共用同一结构）。
type LibraryCreationTask struct {
	TaskID    string   `json:"task_id"`
	Mode      string   `json:"mode"`
	Status    string   `json:"status"`
	Stage     string   `json:"stage,omitempty"`
	Progress  int      `json:"progress"`
	Summary   string   `json:"summary,omitempty"`
	Handles   []string `json:"handles,omitempty"`
	Error     string   `json:"error,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// LibraryCreationResponse 提交 / 查询单个任务的响应。
type LibraryCreationResponse struct {
	OK    bool                 `json:"ok"`
	Task  *LibraryCreationTask `json:"task,omitempty"`
	Error string               `json:"error,omitempty"`
}

// LibraryCreationListResponse 最近任务列表响应。
type LibraryCreationListResponse struct {
	OK    bool                  `json:"ok"`
	Tasks []LibraryCreationTask `json:"tasks,omitempty"`
	Error string                `json:"error,omitempty"`
}

// LibraryStoryboardRequest 演绎提交请求体（WS 命令 storyboard.submit）：故事文本或资源句柄。
type LibraryStoryboardRequest struct {
	// Text 故事正文
	Text string `json:"text"`
	// Handle 故事正文所在的资源句柄（如 @D.001）
	Handle string `json:"handle"`
	// Title 作品标题（可空）
	Title string `json:"title"`
	// Style 统一画风约束（可空）
	Style string `json:"style"`
	// Ratio 统一画幅（可空，默认 16:9）
	Ratio string `json:"ratio"`
}

// LibraryStoryboardShot 分镜执行结果。
type LibraryStoryboardShot struct {
	Index     int    `json:"index"`
	Act       string `json:"act,omitempty"`
	Scene     string `json:"scene"`
	Narration string `json:"narration"`
	Prompt    string `json:"prompt"`
	Ratio     string `json:"ratio,omitempty"`
	Status    string `json:"status"`
	Handle    string `json:"handle,omitempty"`
	File      string `json:"file,omitempty"`
	Error     string `json:"error,omitempty"`
}

// LibraryStoryboardShotInput 审查稿中的单镜输入（确认接口回传）。
type LibraryStoryboardShotInput struct {
	// Index 原稿镜头序号；新增镜头填 0
	Index int `json:"index"`
	// Act 幕名
	Act string `json:"act,omitempty"`
	// Scene 镜次
	Scene string `json:"scene,omitempty"`
	// Narration 本镜叙事
	Narration string `json:"narration,omitempty"`
	// Prompt 画面描述（为空时按叙事重新拼装）
	Prompt string `json:"prompt,omitempty"`
	// Ratio 单镜画幅（为空时用任务默认画幅）
	Ratio string `json:"ratio,omitempty"`
}

// LibraryStoryboardActionRequest 演绎任务控制请求（确认 / 暂停 / 继续 / 终止）。
type LibraryStoryboardActionRequest struct {
	// TaskID 任务标识
	TaskID string `json:"task_id"`
	// Reason 终止原因（可空）
	Reason string `json:"reason,omitempty"`
	// Title 确认时覆盖作品标题（可空）
	Title string `json:"title,omitempty"`
	// Shots 确认时的完整审查稿
	Shots []LibraryStoryboardShotInput `json:"shots,omitempty"`
}

// LibraryStoryboardTask 演绎任务对外状态。
type LibraryStoryboardTask struct {
	TaskID    string                  `json:"task_id"`
	Status    string                  `json:"status"`
	Stage     string                  `json:"stage,omitempty"`
	Progress  int                     `json:"progress"`
	Summary   string                  `json:"summary,omitempty"`
	Title     string                  `json:"title,omitempty"`
	DocPath   string                  `json:"doc_path,omitempty"`
	DocHandle string                  `json:"doc_handle,omitempty"`
	Shots     []LibraryStoryboardShot `json:"shots,omitempty"`
	Handles   []string                `json:"handles,omitempty"`
	Error     string                  `json:"error,omitempty"`
	// Event 最近一次增量事件名，ShotIndex 该事件对应的镜头序号（0 表示不指向具体镜头）。
	Event     string `json:"event,omitempty"`
	ShotIndex int    `json:"shot_index,omitempty"`
	// Thought 规划阶段主智能体的累计可读过程文本。
	Thought string `json:"thought,omitempty"`
	// Seq 同一 epoch 内单调递增的快照序号，前端据此丢弃乱序快照。
	Seq uint64 `json:"seq,omitempty"`
	// Epoch 进程启动标识，重启后前端据此重置本地序号。
	Epoch string `json:"epoch,omitempty"`
	// V 推送协议版本：2 表示增量推送（shot_patch 为差异、shots 为全量）。
	V int `json:"v,omitempty"`
	// ShotPatch 本次推送中发生变化的镜头（仅增量推送出现）。
	ShotPatch []LibraryStoryboardShot `json:"shot_patch,omitempty"`
	// ShotCount 任务当前镜头总数，前端据此裁剪本地多出的镜头。
	ShotCount int    `json:"shot_count,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// LibraryWSEnvelope 资源库 WebSocket 上行命令信封。
//
// 前端只发这一种上行结构；服务端以 {"type":"library_reply","data":{...}}
// 定向回执到发起命令的连接，id 用于把回执与请求配对。
type LibraryWSEnvelope struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// LibraryWSReply 命令回执；Result 为对应动作的响应体（与 HTTP 接口同构）。
type LibraryWSReply struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

// LibraryTaskQuery 按任务标识查询状态 / 列表的通用入参。
type LibraryTaskQuery struct {
	TaskID string `json:"task_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// LibraryStoryboardResponse 提交 / 查询单个任务的响应。
type LibraryStoryboardResponse struct {
	OK    bool                   `json:"ok"`
	Task  *LibraryStoryboardTask `json:"task,omitempty"`
	Error string                 `json:"error,omitempty"`
}

// LibraryStoryboardListResponse 最近演绎任务列表响应。
type LibraryStoryboardListResponse struct {
	OK    bool                    `json:"ok"`
	Tasks []LibraryStoryboardTask `json:"tasks,omitempty"`
	Error string                  `json:"error,omitempty"`
}

// DataDumpKind 数据导出类型
type DataDumpKind string

// DataDumpRequest 一次数据导出请求（TUI 与 HTTP handler 共用同一服务入口）
type DataDumpRequest struct {
	// Kind 导出类型
	Kind DataDumpKind
	// Agent 智能体上下文导出目标（仅 DataDumpContext 使用）
	Agent string
	// IncludeImages 是否保留图片原始 base64（仅 DataDumpContext 使用）
	IncludeImages bool
}

// DBSQLRequest 自然语言转 SQL 请求。
type DBSQLRequest struct {
	Path     string `json:"path"`
	Question string `json:"question"`
}

// DBSQLResponse 自然语言转 SQL 回执：只返回一条只读 SQL，不执行。
type DBSQLResponse struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path"`
	SQL   string `json:"sql"`
	Error string `json:"error,omitempty"`
}
