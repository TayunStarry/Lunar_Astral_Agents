package ManipulateFiles

// tools.go 文件操纵工具：句柄读取、按行替换、追加、删除、创建与删除文件。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/agents/filesupport"
	"LunarSubsystem/LunarHarness/agents/toolagent"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

const (
	toolFetch        = "fetch_resources"
	toolReplace      = "file_replace"
	toolAppend       = "file_append"
	toolDelete       = "file_delete"
	toolCreate       = "file_create"
	toolUninstall    = "file_uninstall"
	maxEvidenceRunes = 1200
	maxEvidenceItems = 8
)

// collector 收集句柄、变更路径与证据。
type collector struct {
	root          string
	handle        []string
	changedPaths  []string
	evidenceItems []string
}

func (c *collector) addHandle(handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || contains(c.handle, handle) {
			continue
		}
		c.handle = append(c.handle, handle)
	}
}

func (c *collector) addChanged(rel string) {
	rel = strings.TrimSpace(rel)
	if rel == "" || contains(c.changedPaths, rel) {
		return
	}
	c.changedPaths = append(c.changedPaths, rel)
}

func (c *collector) addEvidence(text string) {
	text = strings.TrimSpace(text)
	if text == "" || len(c.evidenceItems) >= maxEvidenceItems {
		return
	}
	c.evidenceItems = append(c.evidenceItems, clip(text, maxEvidenceRunes))
}

func (c *collector) handles() []string  { return append([]string(nil), c.handle...) }
func (c *collector) changed() []string  { return append([]string(nil), c.changedPaths...) }
func (c *collector) evidence() []string { return append([]string(nil), c.evidenceItems...) }

func toolHandler(c *collector) toolagent.Handler {
	return func(name string, args map[string]any) (toolagent.Outcome, bool, error) {
		switch name {
		case toolFetch:
			return fetch(args), false, nil
		case toolReplace:
			return replace(args, c), false, nil
		case toolAppend:
			return appendContent(args, c), false, nil
		case toolDelete:
			return deleteLines(args, c), false, nil
		case toolCreate:
			return createFile(args, c), false, nil
		case toolUninstall:
			return uninstall(args, c), false, nil
		default:
			return toolagent.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func fetch(args map[string]any) toolagent.Outcome {
	result := resourcehub.RunArgs(context.Background(), args)
	return toolagent.Outcome{JSON: result.Text, Summary: "已读取资源"}
}

func replace(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolReplace, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "replace"); err != nil {
		return toolagent.Failure(toolReplace, fmt.Errorf("保存版本失败：%w", err))
	}
	content := toolagent.StringArg(args, "content")
	var result FileEditor.OpResult
	var evidence string
	if toolagent.BoolArg(args, "all") {
		result = FileEditor.WriteFile(rel, content)
		evidence = fmt.Sprintf("替换 %s 全部行：\n%s", rel, clip(content, maxEvidenceRunes))
	} else {
		start := toolagent.IntArg(args, "start_line")
		end := toolagent.IntArg(args, "end_line")
		if start < 1 {
			return toolagent.Failure(toolReplace, fmt.Errorf("非 all 替换必须提供 start_line"))
		}
		if end < start {
			end = start
		}
		result = FileEditor.ReplaceAt(rel, start, end, content)
		evidence = fmt.Sprintf("替换 %s 第 %d-%d 行：\n%s", rel, start, end, clip(content, maxEvidenceRunes))
	}
	return mutated(c, rel, result, evidence)
}

func appendContent(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolAppend, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "append"); err != nil {
		return toolagent.Failure(toolAppend, fmt.Errorf("保存版本失败：%w", err))
	}
	existing, readErr := FileEditor.ReadText(rel)
	if readErr != nil {
		return toolagent.Failure(toolAppend, readErr)
	}
	addition := toolagent.StringArg(args, "content")
	if strings.TrimSpace(addition) == "" {
		return toolagent.Failure(toolAppend, fmt.Errorf("追加内容不能为空"))
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	result := FileEditor.WriteFile(rel, existing+addition+"\n")
	evidence := fmt.Sprintf("向 %s 末尾追加：\n%s", rel, clip(addition, maxEvidenceRunes))
	return mutated(c, rel, result, evidence)
}

func deleteLines(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolDelete, err)
	}
	start := toolagent.IntArg(args, "start_line")
	end := toolagent.IntArg(args, "end_line")
	if start < 1 {
		return toolagent.Failure(toolDelete, fmt.Errorf("start_line 必须从 1 开始"))
	}
	if end < start {
		end = start
	}
	if _, err := filesupport.BackupWithOp(rel, "delete_lines"); err != nil {
		return toolagent.Failure(toolDelete, fmt.Errorf("保存版本失败：%w", err))
	}
	content, readErr := FileEditor.ReadText(rel)
	if readErr != nil {
		return toolagent.Failure(toolDelete, readErr)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if start > len(lines) {
		return toolagent.Failure(toolDelete, fmt.Errorf("起始行 %d 超出文件行数 %d", start, len(lines)))
	}
	if end > len(lines) {
		end = len(lines)
	}
	removed := strings.Join(lines[start-1:end], "\n")
	next := append(append([]string(nil), lines[:start-1]...), lines[end:]...)
	result := FileEditor.WriteFile(rel, strings.Join(next, "\n"))
	evidence := fmt.Sprintf("删除 %s 第 %d-%d 行：\n%s", rel, start, end, clip(removed, maxEvidenceRunes))
	return mutated(c, rel, result, evidence)
}

func createFile(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolCreate, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "create"); err != nil {
		return toolagent.Failure(toolCreate, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.CreateFile(rel, "")
	return mutated(c, rel, result, "创建空白文件 "+rel)
}

func uninstall(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolUninstall, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "uninstall"); err != nil {
		return toolagent.Failure(toolUninstall, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.DeleteFile(rel)
	return mutated(c, rel, result, "删除文件 "+rel)
}

func mutated(c *collector, rel string, result FileEditor.OpResult, evidence string) toolagent.Outcome {
	outcome := toolagent.Outcome{JSON: result.JSON(), Summary: result.Summary, Handles: result.Handles}
	if result.OK {
		outcome.Mutated = true
		c.addChanged(rel)
		c.addHandle(result.Handles...)
		c.addEvidence(evidence)
	}
	return outcome
}

func withHandlePaths(instruction string, handles []string) string {
	lines := make([]string, 0, len(handles))
	for _, handle := range handles {
		entry, err := resource.Resolve(handle)
		if err == nil && strings.TrimSpace(entry.BlobPath) != "" {
			lines = append(lines, "- "+handle+" → "+entry.BlobPath)
		} else {
			lines = append(lines, "- "+handle)
		}
	}
	if len(lines) == 0 {
		return instruction
	}
	return instruction + "\n\n【目标句柄】\n" + strings.Join(lines, "\n")
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
