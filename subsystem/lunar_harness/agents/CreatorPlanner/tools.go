package CreatorPlanner

// tools.go 创作规划者的工具声明：schema + 说明 + 所属智能体 ID（creator）。
//
// 按四层定义，「工具声明」归智能体；执行函数仍由创作链（chain/creation）注册实现。
// 链侧只按工具名从 toolreg 派生定义，不再维护硬编码清单。

import (
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID,
		toolDef("web_search",
			"联网搜索：给出查询意图，返回搜索摘要与参考链接。需要外部资料时使用。",
			map[string]any{"query": strProp("搜索查询，尽量具体")}, []string{"query"}),
		schemaDef("fetch_resources",
			resourcehub.ToolDescription(),
			resourcehub.ToolSchema()),
		toolDef("file_list",
			"列出 /document 根目录下的文件与子目录。path 为空表示根目录。",
			map[string]any{"path": strProp("相对 /document 的目录路径（可空）")}, nil),
		toolDef("file_read",
			"读取 /document 下的文本文件内容；大文件用 offset 分批翻页读取。",
			map[string]any{
				"path":      strProp("相对 /document 的文件路径"),
				"offset":    intProp("起始字符偏移（从 0 开始，用于分批读取）", 0, 1<<20),
				"max_chars": intProp("本次返回字符数（默认 4000，最大 8000）", 1, FileReadChunkLimit),
			}, []string{"path"}),
		toolDef("file_create",
			"在 /document 下创建文件；父目录自动创建，已存在时报错（请改用 file_rewrite）。",
			map[string]any{
				"path":    strProp("相对 /document 的文件路径"),
				"content": strProp("文件完整内容"),
			}, []string{"path", "content"}),
		toolDef("file_rewrite",
			"整篇重写 /document 下的文件（不存在则创建）。",
			map[string]any{
				"path":    strProp("相对 /document 的文件路径"),
				"content": strProp("新的完整内容"),
			}, []string{"path", "content"}),
		toolDef("file_remove",
			"移除文件内容：清空文件但保留文件本身。",
			map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
		toolDef("file_delete",
			"删除 /document 下的文件。",
			map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
		toolDef("file_move",
			"移动或重命名 /document 下的文件。",
			map[string]any{
				"path":   strProp("源文件路径"),
				"target": strProp("目标路径"),
			}, []string{"path", "target"}),
		toolDef("file_replace",
			"在文件中替换指定内容（可只替换第一次或全部）。",
			map[string]any{
				"path":    strProp("相对 /document 的文件路径"),
				"search":  strProp("要查找的原文"),
				"replace": strProp("替换后的文本，可为空串表示删除"),
				"all":     boolProp("是否替换全部匹配；默认 false 只替换第一次"),
			}, []string{"path", "search", "replace"}),
		toolDef("file_delete_content",
			"删除文件中指定的内容片段（可只删第一次或全部）。",
			map[string]any{
				"path": strProp("相对 /document 的文件路径"),
				"text": strProp("要删除的原文"),
				"all":  boolProp("是否删除全部匹配；默认 false"),
			}, []string{"path", "text"}),
		toolDef("archive_read",
			"读取压缩包 / GGUF：target 为压缩包路径，entry 为条目名（可空，空则返回结构摘要）。普通文本文件请用 file_read。",
			map[string]any{
				"target": strProp("压缩包或 GGUF 路径（相对 /document 或完整路径）"),
				"entry":  strProp("压缩包内条目名（可空）"),
				"query":  strProp("读取诉求（可空）"),
			}, []string{"target"}),
		toolDef("archive_write",
			"压缩包写入：action=create 用 sources 打包；delete 删除压缩包；add 用 entry + source/content 新增或覆盖条目；remove 移除条目。",
			map[string]any{
				"action": enumProp("要执行的归档写操作", []string{"create", "delete", "add", "remove"}),
				"target": strProp("压缩包路径（.zip / .ltpx）"),
				"sources": map[string]any{
					"type":        "array",
					"description": "create 时要打包的文件 / 目录路径",
					"items":       map[string]any{"type": "string"},
				},
				"entry":   strProp("add / remove 的条目名"),
				"source":  strProp("add 时从本地文件读入的来源路径（与 content 二选一）"),
				"content": strProp("add 时直接写入的文本内容（与 source 二选一）"),
			}, []string{"action", "target"}),
		toolDef("code_index",
			"代码索引：返回目标代码文件的全局变量、全局函数签名、全局类名及各自注释。",
			map[string]any{"path": strProp("相对 /document 的代码文件路径")}, []string{"path"}),
		toolDef("read_dts",
			"查阅内嵌运行时 API 契约：契约按能力拆分为多个 *API.d.ts，统一存放在 engine/runtime/contracts 目录（runtimeAPI / agentAPI / memoryAPI / databaseAPI 等），按需只读所需模块；name 为空时列出清单。",
			map[string]any{"name": strProp("文档名（如 runtimeAPI.d.ts，只写文件名不带目录；为空时列出全部可查阅文档）")}, nil),
		toolDef("change_query",
			"查看文件操作的内容级变更：返回最新 10 次差异块与更早合并成的原始基准；path 为空时返回全局最近变更。",
			map[string]any{
				"path":  strProp("相对 /document 或完整路径（可空）"),
				"limit": intProp("返回的变更次数（默认 10，最大 10）", 1, MaxChangeQuery),
			}, nil),
		toolDef("send_group_text",
			"内容调度：把文本发送到指定群聊。group 可写群号或群名。",
			map[string]any{
				"group": strProp("群号或群名称"),
				"text":  strProp("要发送的文本内容"),
			}, []string{"group", "text"}),
		toolDef("send_group_file",
			"内容调度：把 /document 下的文件发送到指定群聊。",
			map[string]any{
				"group": strProp("群号或群名称"),
				"path":  strProp("相对 /document 的文件路径"),
				"name":  strProp("发送时显示的文件名（可空）"),
			}, []string{"group", "path"}),
		toolDef("run_script",
			"代码运行：依次运行 /document 下的脚本文件；文件头含「//Engine: Lunar Harness」走月华私有 js 运行时，其余 js/html 自动弹出「资源库」前端并在其 iframe 沙箱中运行。默认运行时长上限 120 秒，可用 timeout_sec 或文件首行 `timeout=120s` 覆盖。",
			map[string]any{
				"scripts": map[string]any{
					"type":        "array",
					"description": "按顺序执行的脚本文件路径（相对 /document）",
					"items":       map[string]any{"type": "string"},
				},
				"timeout_sec": intProp("运行时长上限（秒；默认 120，允许 5-3600）", 5, 3600),
			}, []string{"scripts"}),
		toolDef("script_status",
			"查看最近一次脚本运行结果快照。",
			map[string]any{"name": strProp("运行时名；默认监控创作管线")}, nil),
		toolDef("submit_plan",
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
								"enum":        []string{AgentWriter, AgentEngineer, AgentResearcher, AgentRunner},
								"description": "执行该步骤的专属智能体",
							},
							"task": strProp("用自然语言详细描述该步骤的执行目标与操作要求"),
						},
						"required": []string{"agent", "task"},
					},
				},
				"expect": map[string]any{
					"type":        "array",
					"description": "该步骤完成后应满足的客观事实（自动审查用）；不填时按操作类型使用默认预期",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path":         strProp("目标路径（相对 /document 或完整路径）"),
							"exists":       map[string]any{"type": "boolean", "description": "期望操作后是否存在"},
							"contains":     stringArrayProp("期望包含的文本片段"),
							"not_contains": stringArrayProp("期望不包含的文本片段"),
							"min_bytes":    map[string]any{"type": "integer", "description": "期望最小字节数"},
							"max_bytes":    map[string]any{"type": "integer", "description": "期望最大字节数"},
						},
						"required": []string{"path"},
					},
				},
				"memos": map[string]any{
					"type":        "array",
					"description": "本轮遗留的备忘与期望事项",
					"items":       map[string]any{"type": "string"},
				},
			}, []string{"steps"}),
	)
}

// schemaDef 用完整参数 schema 构造一条工具声明（聚合工具统一走这里）。
func schemaDef(name, description string, parameters map[string]any) toolreg.Schema {
	return toolreg.Schema{Name: name, Description: description, Parameters: parameters}
}

// toolDef 用属性表构造一条参数结构固定的工具声明。
func toolDef(name, description string, properties map[string]any, required []string) toolreg.Schema {
	if required == nil {
		required = []string{}
	}
	return toolreg.Schema{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
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

func enumProp(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func stringArrayProp(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}
