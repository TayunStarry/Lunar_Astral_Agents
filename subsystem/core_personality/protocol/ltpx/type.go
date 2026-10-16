package ltpx

import "context"

// ToolDef 聚合工具定义（use_the_program）：工具名 + 功能简述 + 参数 schema。
// Description 内嵌逐行子工具清单；Parameters 为该聚合工具自身的 JSON Schema
// （{tool, instruction}，由琉璃侧 buildUseTheProgram 提供），随定义下发给模型。
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
	AppID       string `json:"app_id,omitempty"`
}

// Status 工具链状态：当前聚合工具（use_the_program）。
// 合并为单进程后不存在「在线/离线」概念：工具提供方与消费方同处一个进程，
// 工具链不可用只可能是宿主未就绪或扫描结果为空，此时 Tool 为 nil。
type Status struct {
	Tool *ToolDef `json:"tool,omitempty"`
}

// Result 工具调用结果
type Result struct {
	Success bool   `json:"success"`
	Text    string `json:"text"`
	Error   string `json:"error"`
}

// EventResult 事件推送（xx事件发生前）的处理结果。
// Returned 为订阅方回执标志，Return 为业务回传数据，Data 为插件改写后的负载。
type EventResult struct {
	Modified bool `json:"modified"`
	Data     any  `json:"data"`
	Returned bool `json:"returned"`
	Return   any  `json:"return"`
}

// Broker 是 LTPX 协调的进程内契约：月华（消费方）只依赖本接口，
// 琉璃（提供方）实现本接口，由装配层注入，从而彻底消除跨服务 HTTP 通道。
type Broker interface {
	// Status 拉取进程内最新聚合工具链（原 /ltpx/tools）
	Status() Status
	// Call 调用工具并等待执行结果（原 /ltpx/call + /ltpx/poll 轮询，现为进程内结果通道）
	Call(ctx context.Context, tool string, args map[string]any) (Result, error)
	// Submit 异步受理工具调用，返回 request_id（供前端包长任务路径）
	Submit(tool string, args map[string]any) (string, error)
	// Await 等待指定 request_id 的结果（进程内结果通道，替代 HTTP 轮询）
	Await(ctx context.Context, requestID string) (Result, error)
	// Event 推送「xx事件发生前」负载并同步取回 LTP9 插件处理结果
	Event(topic string, payload any) EventResult
}
