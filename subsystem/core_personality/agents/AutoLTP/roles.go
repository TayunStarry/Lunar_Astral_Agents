//go:build windows

package AutoLTP

// roleTools 依据角色工具白名单从 allToolDefs 中抽取对应的工具定义子集。
func roleTools(toolNames []string) []ltpToolDef {
	out := []ltpToolDef{}
	for _, n := range toolNames {
		if t, ok := allToolDefs[n]; ok {
			out = append(out, t)
		}
	}
	return out
}
