package FileEditor

// catalog.go 暴露给模型的工具定义目录，以及控制台「工具列表」屏的展示清单。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// definitionsFor 返回全部工具定义（文件编辑智能体的工具集是固定的）。
func definitionsFor() []toolDefinition {
	out := make([]toolDefinition, 0, len(allTools))
	for _, name := range allTools {
		if def, ok := toolDefinitions[name]; ok {
			out = append(out, def)
		}
	}
	return out
}

// toolDefinitions 工具名 → 函数定义。
var toolDefinitions = map[string]toolDefinition{
	ToolList: toolDef(ToolList,
		"列出 /document 文档库下的文件与子目录；path 为空表示根目录。",
		map[string]any{"path": strProp("相对 /document 的目录路径（可空）")}, nil),
	ToolRead: toolDef(ToolRead,
		"读取 /document 下的文本文件；大文件用 offset 分批读取。",
		map[string]any{
			"path":      strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）"),
			"offset":    intProp("起始字符偏移（默认 0）", 0, 1<<20),
			"max_chars": intProp("本次返回字符数（默认 4000，最大 8000）", 1, fileReadChunkLimit),
		}, []string{"path"}),
	ToolWrite: toolDef(ToolWrite,
		"写入 /document 下文件的完整内容（不存在则创建）；整篇覆盖前请先 file_read。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）"),
			"content": strProp("完整文件内容"),
		}, []string{"path", "content"}),
	ToolCreate: toolDef(ToolCreate,
		"新建 /document 下的文件（已存在时报错，请改用 file_write）。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）"),
			"content": strProp("文件完整内容"),
		}, []string{"path", "content"}),
	ToolDelete: toolDef(ToolDelete,
		"删除 /document 下的文件。",
		map[string]any{"path": strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）")}, []string{"path"}),
	ToolCopy: toolDef(ToolCopy,
		"复制文件到新路径（目标必须不存在）。",
		map[string]any{
			"path":   strProp("源文件路径"),
			"target": strProp("目标文件路径"),
		}, []string{"path", "target"}),
	ToolMove: toolDef(ToolMove,
		"移动文件到新路径（也可用于跨目录改名）。",
		map[string]any{
			"path":   strProp("源文件路径"),
			"target": strProp("目标文件路径"),
		}, []string{"path", "target"}),
	ToolRename: toolDef(ToolRename,
		"同目录内重命名文件（只改文件名，不含路径分隔符）。",
		map[string]any{
			"path": strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）"),
			"name": strProp("新的文件名（含扩展名）"),
		}, []string{"path", "name"}),
	ToolSearch: toolDef(ToolSearch,
		"在文件或目录内检索内容（目录会递归扫描文本文件）；返回路径、行号与命中行。",
		map[string]any{
			"path":  strProp("相对 /document 的文件或目录路径"),
			"query": strProp("要检索的内容"),
		}, []string{"path", "query"}),
	ToolReplace: toolDef(ToolReplace,
		"置换文件指定位置的内容：用 content 替换第 start_line 到 end_line 行（1 基、含端点，行数可增减）。",
		map[string]any{
			"path":       strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）"),
			"start_line": intProp("起始行（1 基）", 1, 1<<20),
			"end_line":   intProp("结束行（含端点；等于 start_line 即单行替换）", 1, 1<<20),
			"content":    strProp("替换后的纯文本内容（不带行号前缀）"),
		}, []string{"path", "start_line", "content"}),
	ToolInfo: toolDef(ToolInfo,
		"查看文件属性：大小、行数、字符数、修改时间、扩展名与资源句柄。",
		map[string]any{"path": strProp("相对 /document 的文件路径（也可写完整路径或纯文件名）")}, []string{"path"}),
	ToolWeb: toolDef(ToolWeb,
		"联网查询：给出查询意图，返回搜索摘要。需要外部资料时使用。",
		map[string]any{"query": strProp("搜索查询，尽量具体")}, []string{"query"}),
	ToolRunProj: toolDef(ToolRunProj,
		"运行 js / html 项目文件：文件头含「//Engine: Lunar Harness」走月华私有 js 运行时；其余 js/html 自动弹出「资源库」前端并在其 iframe 沙箱中运行。默认运行时长上限 120 秒，可用 timeout_sec 或文件首行 `timeout=120s` 覆盖。",
		map[string]any{
			"path":        strProp("相对 /document 的项目文件路径"),
			"timeout_sec": intProp("运行时长上限（秒；默认 120，允许 5-3600；优先级高于文件首行声明）", 5, 3600),
		}, []string{"path"}),
	ToolSchedule: toolDef(ToolSchedule,
		"单方面调度其他智能体完成任务：type 选定目标智能体，instruction 用自然语言说明要做什么；target 可选（路径或句柄，缺省时由本智能体从 instruction 中提取）。压缩包、GGUF、文档正文、图片 / 动图 / 视频一律走这里，不要用文件工具直接读写二进制容器。",
		map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{ScheduleArchiveOps, ScheduleArchiveReader, ScheduleDocumentReader, ScheduleVision},
				"description": "archive_ops=归档操作（创建/删除压缩包、增删条目）；archive_reader=归档解析（压缩包结构/条目/GGUF）；document_reader=文献解析（理解文档正文）；vision=视觉理解（图片/动图/视频）",
			},
			"instruction": strProp("自然语言任务描述，写清要做什么、对哪个文件或句柄做什么"),
			"target":      strProp("可选：目标路径或句柄；缺省时从 instruction 中提取"),
		}, []string{"type", "instruction"}),
}

// toolDef 构造一条函数工具定义。
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

func intProp(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}

// ToolCatalog 返回文件编辑智能体的工具清单（供控制台「工具列表」屏展示）。
func ToolCatalog() []agentinfo.Agent {
	tools := make([]agentinfo.Tool, 0, len(allTools))
	for _, name := range allTools {
		if def, ok := toolDefinitions[name]; ok {
			tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
		}
	}
	return []agentinfo.Agent{{
		ID: "file_editor", Name: "文件编辑", Source: "agents/FileEditor",
		Note:  "文件编辑智能体：文档库读写、检索、置换与项目运行。",
		Tools: tools,
	}}
}
