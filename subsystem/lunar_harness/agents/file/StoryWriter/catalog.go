package StoryWriter

// catalog.go 故事编写工具声明与目录展示。

import (
	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var toolDefinitions = []agentbase.ToolDefinition{
	{
		Type: "function",
		Function: agentbase.ToolFunctionDef{
			Name:        toolFetch,
			Description: resourcehub.ToolDescription(),
			Parameters:  resourcehub.ToolSchema(),
		},
	},
	file.Def(toolStories,
		"列出 documents 下已有故事（包含 大纲.md 的目录），按最近更新倒序；新建前确认标题不重复，延续时确认故事名。",
		map[string]any{}, nil),
	file.Def(toolList,
		"列出故事目录下的文件；path 填故事名（如 `雨夜旧书店`），留空则列出 documents 根目录。",
		map[string]any{"path": file.StrProp("故事目录或相对 documents 的路径")}, nil),
	file.Def(toolNew,
		"新建故事：由系统在 documents/<标题>/ 下创建 大纲.md 与 进度.md。标题由系统判定时以系统标题为准；故事已存在会失败。",
		map[string]any{
			"title":   file.StrProp("故事标题，不超过 16 个汉字，不能与已有故事重名"),
			"outline": file.StrProp("完整 Markdown 大纲：一级标题用《标题》，包含 背景设定 / 人物信息 / 章节纲要 三节"),
		}, []string{"title", "outline"}),
	file.Def(toolChapter,
		"写一章正文：由系统拼出 documents/<故事>/NNN-章节名.md 并自动追加进度。不要自己拼路径，也不要用 file_create 写章节。",
		map[string]any{
			"story":   file.StrProp("故事目录名；延续时系统会强制为当前故事，可留空"),
			"number":  file.IntProp("章节编号，1 表示 001；留空自动取下一号", 1, 999),
			"title":   file.StrProp("章节名，不含编号和扩展名，如 `无人小船`"),
			"content": file.StrProp("本章正文，3000～5000 汉字，只写正文"),
			"summary": file.StrProp("本章关键事件，用于自动写入进度"),
			"changes": file.StrProp("本章带来的设定变更；没有写「无」"),
		}, []string{"title", "content", "summary"}),
	file.Def(toolCreate,
		"创建非章节的辅助文件（已存在则失败，父目录会自动创建）。章节正文必须用 write_chapter。",
		map[string]any{
			"path":    file.StrProp("文件路径，相对 documents，如 `故事名/备注.md`"),
			"content": file.StrProp("完整文件内容"),
		}, []string{"path", "content"}),
	file.Def(toolAppend,
		"在已有文件末尾追加内容。用于向 大纲.md 追加章节纲要；进度由 write_chapter 自动维护，不要手动追加。",
		map[string]any{
			"path":    file.StrProp("文件路径，如 `故事名/大纲.md`"),
			"content": file.StrProp("要追加的内容"),
		}, []string{"path", "content"}),
}

func init() {
	for _, def := range toolDefinitions {
		toolreg.Register(AgentID, toolreg.Schema{
			Name:        def.Function.Name,
			Description: def.Function.Description,
			Parameters:  def.Function.Parameters,
		})
	}
}

func definitions() []agentbase.ToolDefinition {
	return append([]agentbase.ToolDefinition(nil), toolDefinitions...)
}

// ToolCatalog 返回故事编写智能体的工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{ID: AgentID, Name: DisplayName, Tools: agentinfo.ToolsFromRegistry(AgentID)}}
}
