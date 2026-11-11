package Screenshoter

// tools.go 截图者工具声明。

import "LunarSubsystem/LunarHarness/protocol/agentbase"

// toolDefinitions 截图者工具集。
func toolDefinitions() []agentbase.ToolDefinition {
	return []agentbase.ToolDefinition{
		functionTool("list_capture_targets",
			"列出当前所有显示器与可见窗口（标题、进程、窗口矩形、客户区尺寸、是否最小化、是否焦点）。",
			map[string]any{"include_minimized": map[string]any{"type": "boolean", "description": "是否也列出已最小化的窗口"}},
			[]string{}),
		functionTool("capture_screen",
			"执行一次截图并立即展示给用户。mode=auto/window/fullscreen/display/region。",
			map[string]any{
				"mode":          map[string]any{"type": "string", "enum": []string{"auto", "window", "fullscreen", "display", "region"}, "description": "截图模式，默认 auto"},
				"window_title":  map[string]any{"type": "string", "description": "目标窗口标题关键字"},
				"window_handle": map[string]any{"type": "number", "description": "目标窗口句柄"},
				"client_area":   map[string]any{"type": "boolean", "description": "是否只截客户区"},
				"display_index": map[string]any{"type": "number", "description": "显示器索引"},
				"offset_x":      map[string]any{"type": "number", "description": "窗口内相对 X 偏移"},
				"offset_y":      map[string]any{"type": "number", "description": "窗口内相对 Y 偏移"},
				"width":         map[string]any{"type": "number", "description": "窗口内区域宽度"},
				"height":        map[string]any{"type": "number", "description": "窗口内区域高度"},
				"region_x":      map[string]any{"type": "number", "description": "绝对屏幕区域 X"},
				"region_y":      map[string]any{"type": "number", "description": "绝对屏幕区域 Y"},
				"region_w":      map[string]any{"type": "number", "description": "绝对屏幕区域宽度"},
				"region_h":      map[string]any{"type": "number", "description": "绝对屏幕区域高度"},
				"format":        map[string]any{"type": "string", "enum": []string{"png", "jpg"}, "description": "输出格式"},
				"note":          map[string]any{"type": "string", "description": "本次截图的一句话说明"},
			}, []string{}),
	}
}

// functionTool 组装一个 function 工具声明。
func functionTool(name, description string, properties map[string]any, required []string) agentbase.ToolDefinition {
	return agentbase.ToolDefinition{
		Type: "function",
		Function: agentbase.ToolFunctionDef{
			Name:        name,
			Description: description,
			Parameters: map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}
