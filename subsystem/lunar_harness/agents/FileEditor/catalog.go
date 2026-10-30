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
			"path":      strProp("相对 /document 的文件路径"),
			"offset":    intProp("起始字符偏移（默认 0）", 0, 1<<20),
			"max_chars": intProp("本次返回字符数（默认 4000，最大 8000）", 1, fileReadChunkLimit),
		}, []string{"path"}),
	ToolWrite: toolDef(ToolWrite,
		"写入 /document 下文件的完整内容（不存在则创建）；整篇覆盖前请先 file_read。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径"),
			"content": strProp("完整文件内容"),
		}, []string{"path", "content"}),
	ToolCreate: toolDef(ToolCreate,
		"新建 /document 下的文件（已存在时报错，请改用 file_write）。",
		map[string]any{
			"path":    strProp("相对 /document 的文件路径"),
			"content": strProp("文件完整内容"),
		}, []string{"path", "content"}),
	ToolDelete: toolDef(ToolDelete,
		"删除 /document 下的文件。",
		map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
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
			"path": strProp("相对 /document 的文件路径"),
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
			"path":       strProp("相对 /document 的文件路径"),
			"start_line": intProp("起始行（1 基）", 1, 1<<20),
			"end_line":   intProp("结束行（含端点；等于 start_line 即单行替换）", 1, 1<<20),
			"content":    strProp("替换后的纯文本内容（不带行号前缀）"),
		}, []string{"path", "start_line", "content"}),
	ToolInfo: toolDef(ToolInfo,
		"查看文件属性：大小、行数、字符数、修改时间、扩展名与资源句柄。",
		map[string]any{"path": strProp("相对 /document 的文件路径")}, []string{"path"}),
	ToolWeb: toolDef(ToolWeb,
		"联网查询：给出查询意图，返回搜索摘要。需要外部资料时使用。",
		map[string]any{"query": strProp("搜索查询，尽量具体")}, []string{"query"}),
	ToolMemory: toolDef(ToolMemory,
		"记忆库查询：检索历史记忆中的相关片段。",
		map[string]any{
			"query": strProp("检索关键词或自然语言问题"),
			"limit": intProp("返回条数（1-20，默认 8）", 1, 20),
		}, []string{"query"}),
	ToolHandleSr: toolDef(ToolHandleSr,
		"句柄系统检索：按关键词查找已登记的资源句柄与摘要。",
		map[string]any{
			"query": strProp("检索关键词"),
			"limit": intProp("返回条数（1-20，默认 8）", 1, 20),
		}, []string{"query"}),
	ToolHandleRd: toolDef(ToolHandleRd,
		"句柄系统读取：按资源句柄（如 @D.003）读取内容。",
		map[string]any{
			"handle":    strProp("资源句柄"),
			"max_bytes": intProp("读取上限字节（默认 65536）", 1024, maxFileReadBytes),
		}, []string{"handle"}),
	ToolRunProj: toolDef(ToolRunProj,
		"运行 js / html 项目文件：文件头含「//Engine: Lunar Harness」走月华私有 js 运行时；其余 js/html 走浏览器运行环境（纯 js 自动生成可展示结果的模板网页）。默认运行时长上限 120 秒，可用 timeout_sec 或文件首行 `timeout=120s` 覆盖。",
		map[string]any{
			"path":        strProp("相对 /document 的项目文件路径"),
			"timeout_sec": intProp("运行时长上限（秒；默认 120，允许 5-3600；优先级高于文件首行声明）", 5, 3600),
		}, []string{"path"}),
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
