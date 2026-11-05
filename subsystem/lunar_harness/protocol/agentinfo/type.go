package agentinfo

// type.go Go 智能体工具目录的公共类型。
//
// 这个包故意不依赖任何业务包或 TUI 包：智能体包负责描述自己的工具，
// 控制台只读取并展示，宿主负责把它们汇总后注入控制台。

// Tool 一个可展示的工具条目
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Agent 一个具备工具的条目及其工具清单；Kind 区分智能体 / 链 / 执行体。
type Agent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`         // 代码位置，如 lunar / agents/Vision
	Kind   string `json:"kind,omitempty"` // agent / chain / executor，空值等价于 agent
	Note   string `json:"note,omitempty"`
	Tools  []Tool `json:"tools"`
}
