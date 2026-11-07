package agentrepair

// type.go 智能体诊断信息的公共类型。
//
// 智能维修只「看」不「改」：各包在 init() 里注册一个只读处理器，
// 维修侧通过注册表读取目标配置与运行态，但没有任何执行修复动作的入口。
// 这样维修结论只能是建议，不会直接改变其它智能体的状态或行为。

// Handler 一个可观察目标
type Handler struct {
	ID          string // 目标标识（与 agentdump 的 ID 尽量一致）
	Label       string // 显示名
	Description string // 说明
	// Inspect 读取上下文快照；limit<=0 表示全部
	Inspect func(limit int) (any, error)
	// Status 读取配置与运行态（模型、提示词、水位、最近工具序列等）；可省略
	Status func() (any, error)
}

// Info 注册表对外暴露的只读信息
type Info struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	HasStatus   bool   `json:"has_status"`
}
