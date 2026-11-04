package creation

// tools_catalog.go 创作体系的工具定义：从 agents/CreatorPlanner 在 protocol/toolreg
// 登记的声明派生，链侧不再维护硬编码的工具 schema。

import (
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// definitionsFor 返回指定工具名对应的函数定义（按传入顺序，缺失的工具跳过）。
func definitionsFor(names []string) []toolDefinition {
	out := make([]toolDefinition, 0, len(names))
	for _, name := range names {
		schema, ok := toolreg.LookupSchema(name)
		if !ok {
			continue
		}
		out = append(out, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name:        schema.Name,
				Description: schema.Description,
				Parameters:  schema.Parameters,
			},
		})
	}
	return out
}
