package creation

// tools_files.go 创作管线的只读分析工具（代码索引与 DTS 文档查阅）。
// 文件读写 / 压缩包 / 大型目录等操作统一由 flow.go 的路由执行器承接，
// 本文件不再直接定义文件操作实现，也不直接调度文件编辑智能体。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
)

// fileOutcome 把文件编辑智能体的结构化结果转成创作管线的工具回填。
// JSON 里已包含 handles（本次改动涉及的资源句柄）与 summary（本次操作简述）。
func fileOutcome(result FileEditor.OpResult) toolOutcome {
	return toolOutcome{JSON: result.JSON(), Summary: result.Summary}
}

func codeIndexTool(args map[string]any) toolOutcome {
	path := argString(args, "path")
	content, err := FileEditor.ReadText(path)
	if err != nil {
		return failure(toolCodeIndex, err)
	}
	index := buildCodeIndex(content)
	return toolOutcome{
		JSON:    jsonText(index),
		Summary: fmt.Sprintf("代码索引：%d 变量 / %d 函数 / %d 类", len(index.Variables), len(index.Functions), len(index.Classes)),
	}
}

func readDTSTool(args map[string]any) toolOutcome {
	name := strings.TrimSpace(argString(args, "name"))
	content, err := readDTSDoc(name)
	if err != nil {
		return failure(toolReadDTS, err)
	}
	return toolOutcome{
		JSON:      jsonText(map[string]any{"name": name, "content": content}),
		Summary:   "查阅 DTS 文档 " + previewRunes(name, 512),
		Reference: "DTS 文档《" + previewRunes(name, 512) + "》：" + content,
	}
}
