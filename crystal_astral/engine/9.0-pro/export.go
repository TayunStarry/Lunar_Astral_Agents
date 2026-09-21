package ltp9

// ==== 宿主 facade 导出（供宿主包枚举/查询工具与插件） ====

import "sort"

// ToolDefInfo 单工具的只读定义，供宿主枚举工具。
type ToolDefInfo struct {
	PluginID    string
	Name        string
	Description string
	Parameters  []any
}

// PluginToolDefs 返回 插件ID → 工具定义 映射，仅统计已加载插件。
func PluginToolDefs() map[string][]ToolDefInfo {
	out := map[string][]ToolDefInfo{}
	if Engine == nil {
		return out
	}
	for _, p := range Engine.snapshotPlugins() {
		p.mu.Lock()
		if !p.loaded {
			p.mu.Unlock()
			continue
		}
		list := make([]ToolDefInfo, 0, len(p.tools))
		for _, t := range p.tools {
			list = append(list, ToolDefInfo{
				PluginID:    p.ID,
				Name:        t.name,
				Description: t.description,
				Parameters:  append([]any(nil), t.parameters...),
			})
		}
		p.mu.Unlock()
		if len(list) > 0 {
			out[p.ID] = list
		}
	}
	return out
}

// FindPluginByToolName 返回注册了指定工具名的插件 ID（仅在已加载插件中查找）。
func FindPluginByToolName(toolName string) (string, bool) {
	if Engine == nil || toolName == "" {
		return "", false
	}
	for _, p := range Engine.snapshotPlugins() {
		p.mu.Lock()
		_, ok := p.tools[toolName]
		loaded := p.loaded
		p.mu.Unlock()
		if ok && loaded {
			return p.ID, true
		}
	}
	return "", false
}

// IsPluginLoaded 判断插件当前是否处于已加载状态。
func IsPluginLoaded(pluginID string) bool {
	if Engine == nil || pluginID == "" {
		return false
	}
	p := Engine.pluginFor(pluginID)
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loaded
}

// PluginToolNames 返回某插件注册的工具名（仅统计已加载插件）。
func PluginToolNames(pluginID string) []string {
	if Engine == nil || pluginID == "" {
		return []string{}
	}
	p := Engine.pluginFor(pluginID)
	if p == nil {
		return []string{}
	}
	p.mu.Lock()
	if !p.loaded {
		p.mu.Unlock()
		return []string{}
	}
	names := make([]string, 0, len(p.tools))
	for n := range p.tools {
		names = append(names, n)
	}
	p.mu.Unlock()
	sort.Strings(names)
	return names
}

// PluginConfigSchema 返回某插件自声明的配置 schema（engine.config.registerSchema）。
func PluginConfigSchema(pluginID string) []ConfigSectionDef {
	if Engine == nil || pluginID == "" {
		return nil
	}
	p := Engine.pluginFor(pluginID)
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded {
		return nil
	}
	out := make([]ConfigSectionDef, len(p.configSchema))
	copy(out, p.configSchema)
	return out
}