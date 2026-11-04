package agentinfo

// registry.go 工具目录的注册表派生：阶段能力统一从 protocol/toolreg 读取，
// 智能体与链不再各自维护硬编码清单。

import (
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// ToolsFromRegistry 返回若干智能体在 toolreg 登记的阶段能力。
// 顺序按传入的拥有者顺序排列；同一拥有者内按名称排序（toolreg.Schemas 的稳定顺序）。
func ToolsFromRegistry(owners ...string) []Tool {
	out := make([]Tool, 0, len(owners))
	seen := make(map[string]bool)
	for _, owner := range owners {
		for _, schema := range toolreg.Schemas(owner) {
			if schema.Name == "" || seen[schema.Name] {
				continue
			}
			seen[schema.Name] = true
			out = append(out, Tool{Name: schema.Name, Description: schema.Description})
		}
	}
	return out
}
