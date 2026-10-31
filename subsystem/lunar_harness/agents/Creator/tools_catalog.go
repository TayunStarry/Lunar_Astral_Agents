package Creator

// tools_catalog.go 创作体系暴露给模型的工具定义目录。
// 工具分为：联网搜索、记忆库查询、句柄系统、文件操作、内容调度、
// 代码运行（LTP9）、DTS 开发文档查阅，以及调度者的计划提交。

// definitionsFor 返回指定工具名对应的函数定义。
func definitionsFor(names []string) []toolDefinition {
	out := make([]toolDefinition, 0, len(names))
	for _, name := range names {
		if def, ok := toolDefinitions[name]; ok {
			out = append(out, def)
		}
	}
	return out
}

// toolDefinitions 工具名 → 函数定义。
var toolDefinitions = map[string]toolDefinition{
	toolWebSearch: toolDef(toolWebSearch,
		"联网搜索：给出查询意图，返回搜索摘要与参考链接。需要外部资料时使用。",
		map[string]any{"query": strProp("搜索查询，尽量具体")}, []string{"query"}),
	toolMemorySearch: toolDef(toolMemorySearch,
		"记忆库查询：检索历史记忆中的相关片段。",
		map[string]any{
			"query": strProp("检索关键词或自然语言问题"),
			"limit": intProp("返回条数（1-20，默认 8）", 1, 20),
		}, []string{"query"}),
	toolHandleSearch: toolDef(toolHandleSearch,
		"句柄系统检索：按关键词查找已登记的资源句柄与摘要。",
		map[string]any{
			"query": strProp("检索关键词"),
			"limit": intProp("返回条数（1-20，默认 8）", 1, 20),
		}, []string{"query"}),
	toolHandleRead: toolDef(toolHandleRead,
		"句柄系统读取：按资源句柄（如 @D.003）读取内容。",
		map[string]any{
			"handle":    strProp("资源句柄"),
			"max_bytes": intProp("读取上限字节（默认 65536）", 1024, maxFileReadBytes),
		}, []string{"handle"}),
	toolFileList: toolDef(toolFileList,
		"列出 /document 根目录下的文件与子目录。path 为空表示根目录。",
		map[string]any{"path": strProp("相对 /document 的目录路径（可空）")}, nil),
	toolFileRead: toolDef(toolFileRead,
		"读取 /document 下的文本文件内容；大文件用 offset 分批翻页读取。",
		map[string]any{
			"path":      strProp("相对 /document 的文件路径"),
			"offset":    intProp("起始字符偏移（从 0 开始，用于分批读取）", 0, 1<<20),
			"max_chars": intProp("本次返回字符数（默认 4000，最大 8000）", 1, fileReadChunkLimit),
		}, []string{"path"}),
	toolFileCreate: toolDef(toolFileCreate,
		"在 /document 下创建文件；父目录自动创建，已存在时报错（请改用 file_rewrite）。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径"),
			"content": strProp("文件完整内容"),
		}, []string{"path", "content"}),
	toolFileRewrite: toolDef(toolFileRewrite,
		"整篇重写 /document 下的文件（不存在则创建）。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径"),
			"content": strProp("新的完整内容"),
		}, []string{"path", "content"}),
	toolFileRemove: toolDef(toolFileRemove,
		"移除文件内容：清空文件但保留文件本身。",
		map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
	toolFileDelete: toolDef(toolFileDelete,
		"删除 /document 下的文件。",
		map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
	toolFileMove: toolDef(toolFileMove,
		"移动或重命名 /document 下的文件。",
		map[string]any{
			"path":   strProp("源文件路径"),
			"target": strProp("目标路径"),
		}, []string{"path", "target"}),
	toolFileReplace: toolDef(toolFileReplace,
		"在文件中替换指定内容（可只替换第一次或全部）。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径"),
			"search":  strProp("要查找的原文"),
			"replace": strProp("替换后的文本，可为空串表示删除"),
			"all":     boolProp("是否替换全部匹配；默认 false 只替换第一次"),
		}, []string{"path", "search", "replace"}),
	toolFileDeleteText: toolDef(toolFileDeleteText,
		"删除文件中指定的内容片段（可只删第一次或全部）。",
		map[string]any{
			"path": strProp("相对 /document 的文件路径"),
			"text": strProp("要删除的原文"),
			"all":  boolProp("是否删除全部匹配；默认 false"),
		}, []string{"path", "text"}),
	toolFileEdit: toolDef(toolFileEdit,
		"文件编辑智能体：用自然语言描述一组文件操作需求（读写 / 检索 / 置换 / 运行），由专用智能体规划并执行；返回结构化结果（含资源句柄与操作简述）。",
		map[string]any{"instruction": strProp("自然语言操作要求，例如「把 notes/a.md 第 2 行改为月华，并运行 scripts/demo.js」")},
		[]string{"instruction"}),
	toolCodeIndex: toolDef(toolCodeIndex,
		"代码索引：返回目标代码文件的全局变量、全局函数签名、全局类名及各自注释。",
		map[string]any{"path": strProp("相对 /document 的代码文件路径")}, []string{"path"}),
	toolReadDTS: toolDef(toolReadDTS,
		"查阅内嵌运行时 API 契约：契约按能力拆分为多个 *API.d.ts（runtimeAPI / agentAPI / memoryAPI / databaseAPI 等），按需只读所需模块；name 为空时列出清单。",
		map[string]any{"name": strProp("文档名（如 runtimeAPI.d.ts；为空时列出全部可查阅文档）")}, nil),
	toolSendGroupText: toolDef(toolSendGroupText,
		"内容调度：把文本发送到指定群聊。group 可写群号或群名。",
		map[string]any{
			"group": strProp("群号或群名称"),
			"text":  strProp("要发送的文本内容"),
		}, []string{"group", "text"}),
	toolSendGroupFile: toolDef(toolSendGroupFile,
		"内容调度：把 /document 下的文件发送到指定群聊。",
		map[string]any{
			"group": strProp("群号或群名称"),
			"path":  strProp("相对 /document 的文件路径"),
			"name":  strProp("发送时显示的文件名（可空）"),
		}, []string{"group", "path"}),
	toolRunScript: toolDef(toolRunScript,
		"代码运行：依次运行 /document 下的脚本文件；文件头含「//Engine: Lunar Harness」走月华私有 js 运行时，其余 js/html 走浏览器运行环境（纯 js 自动生成可展示结果的模板网页）。默认运行时长上限 120 秒，可用 timeout_sec 或文件首行 `timeout=120s` 覆盖。",
		map[string]any{
			"scripts": map[string]any{
				"type":        "array",
				"description": "按顺序执行的脚本文件路径（相对 /document）",
				"items":       map[string]any{"type": "string"},
			},
			"timeout_sec": intProp("运行时长上限（秒；默认 120，允许 5-3600）", 5, 3600),
		}, []string{"scripts"}),
	toolScriptStatus: toolDef(toolScriptStatus,
		"查看最近一次脚本运行结果快照。",
		map[string]any{"name": strProp("运行时名；默认监控创作管线")}, nil),
	toolSubmitPlan: toolDef(toolSubmitPlan,
		"提交本轮创作计划：steps 必须按严格顺序排列，每项用自然语言写清执行目标与操作要求；memos 记录本轮遗留的备忘与期望。",
		map[string]any{
			"steps": map[string]any{
				"type":        "array",
				"description": "按执行顺序排列的调度项",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent": map[string]any{
							"type":        "string",
							"enum":        []string{agentWriter, agentEngineer, agentResearcher, agentRunner},
							"description": "执行该步骤的专属智能体",
						},
						"task": strProp("用自然语言详细描述该步骤的执行目标与操作要求"),
					},
					"required": []string{"agent", "task"},
				},
			},
			"memos": map[string]any{
				"type":        "array",
				"description": "本轮遗留的备忘与期望事项",
				"items":       map[string]any{"type": "string"},
			},
		}, []string{"steps"}),
}

func toolDef(name, description string, properties map[string]any, required []string) toolDefinition {
	if required == nil {
		required = []string{}
	}
	return toolDefinition{
		Type: "function",
		Function: toolFunction{
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

func strProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func intProp(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}
