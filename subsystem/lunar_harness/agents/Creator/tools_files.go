package Creator

// tools_files.go 文件类工具：能力实现已迁移至「文件编辑」智能体（agents/FileEditor），
// 这里只保留创作管线自己的代码索引与 DTS 文档查阅（纯分析，不改文件）。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
)

// fileEditTool 以自然语言把一组文件操作需求交给「文件编辑」智能体执行。
func fileEditTool(args map[string]any) toolOutcome {
	instruction := strings.TrimSpace(argString(args, "instruction"))
	if instruction == "" {
		return failure(toolFileEdit, fmt.Errorf("instruction 不能为空"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := FileEditor.Execute(ctx, instruction)
	if err != nil {
		return failure(toolFileEdit, err)
	}
	return fileOutcome(result)
}

// fileOutcome 把文件编辑智能体的结构化结果转成创作管线的工具回填。
// JSON 里已包含 handles（本次改动涉及的资源句柄）与 summary（本次操作简述）。
func fileOutcome(result FileEditor.OpResult) toolOutcome {
	return toolOutcome{JSON: result.JSON(), Summary: result.Summary}
}

func fileList(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.FileList(argString(args, "path")))
}

func fileRead(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.ReadFile(
		argString(args, "path"),
		argInt(args, "offset", 0),
		argInt(args, "max_chars", 0),
	))
}

func fileCreate(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.CreateFile(argString(args, "path"), argString(args, "content")))
}

func fileRewrite(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.WriteFile(argString(args, "path"), argString(args, "content")))
}

func fileRemove(args map[string]any) toolOutcome {
	// 清空内容：用空内容整篇写入（文件保留）
	return fileOutcome(FileEditor.WriteFile(argString(args, "path"), ""))
}

func fileDelete(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.DeleteFile(argString(args, "path")))
}

func fileMove(args map[string]any) toolOutcome {
	return fileOutcome(FileEditor.MoveFile(argString(args, "path"), argString(args, "target")))
}

func fileReplace(args map[string]any) toolOutcome {
	// 原文替换：按行检索命中行后整行置换，保持「替换指定内容」的语义
	path := argString(args, "path")
	search := argString(args, "search")
	replace := argString(args, "replace")
	if strings.TrimSpace(search) == "" {
		return failure(toolFileReplace, fmt.Errorf("要查找的原文不能为空"))
	}
	content, err := FileEditor.ReadText(path)
	if err != nil {
		return failure(toolFileReplace, err)
	}
	if !strings.Contains(content, search) {
		return failure(toolFileReplace, fmt.Errorf("未找到要替换的内容"))
	}
	next := content
	if argBool(args, "all") {
		next = strings.ReplaceAll(content, search, replace)
	} else {
		next = strings.Replace(content, search, replace, 1)
	}
	return fileOutcome(FileEditor.WriteFile(path, next))
}

func fileDeleteText(args map[string]any) toolOutcome {
	path := argString(args, "path")
	text := argString(args, "text")
	if strings.TrimSpace(text) == "" {
		return failure(toolFileDeleteText, fmt.Errorf("要删除的原文不能为空"))
	}
	content, err := FileEditor.ReadText(path)
	if err != nil {
		return failure(toolFileDeleteText, err)
	}
	if !strings.Contains(content, text) {
		return failure(toolFileDeleteText, fmt.Errorf("未找到要删除的内容"))
	}
	next := strings.Replace(content, text, "", 1)
	if argBool(args, "all") {
		next = strings.ReplaceAll(content, text, "")
	}
	return fileOutcome(FileEditor.WriteFile(path, next))
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
		Summary:   "查阅 DTS 文档 " + previewRunes(name, 40),
		Reference: "DTS 文档《" + previewRunes(name, 60) + "》：" + content,
	}
}
