package agentrepair

// type.go 智能体上下文修复的公共类型。
//
// 拯救者需要「读上下文 + 改上下文」两件事。具体智能体的上下文只在自己包里，
// 因此各包在 init() 里注册一个修复处理器，拯救者通过注册表统一访问，
// 不需要认识任何具体智能体。

// Handler 一个可修复目标
type Handler struct {
	ID          string   // 目标标识（与 agentdump 的 ID 尽量一致）
	Label       string   // 显示名
	Description string   // 说明
	Actions     []string // 支持的修复动作
	Inspect     func(limit int) (any, error)
	Repair      func(action string, params map[string]any) (string, error)
}

// Info 注册表对外暴露的只读信息
type Info struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Actions     []string `json:"actions"`
}
