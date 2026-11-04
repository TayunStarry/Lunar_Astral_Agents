package FileEditor

// agent.go 文件编辑智能体对外入口：自然语言指令 → 工具循环 → 中文简述。
//
// 除自然语言入口外，本包还导出逐项能力（ReadFile / WriteFile / …），
// 供「月华-生活系统-创作」在同一进程内直接调度，返回值同样带句柄与操作简述。

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Run 用自然语言发布一次文件编辑需求，返回本轮操作简述。
func Run(ctx context.Context, instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errEmptyInstruction
	}
	runMu.Lock()
	defer runMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	summary, _, err := runToolLoop(runCtx, systemPrompt(), instruction)
	if err != nil {
		logger.Warn(Module, "[文件编辑] 执行失败: %v", err)
		agentbase.ReportRun("file_editor", err)
		return "", err
	}
	if summary == "" {
		summary = "已完成文件编辑操作"
	}
	logger.Info(Module, "[文件编辑] 本轮完成：%s", previewRunes(summary, 512))
	return summary, nil
}

// systemPrompt 文件编辑智能体的系统提示（统一收纳在 prompts/fileEditor.md）。
func systemPrompt() string {
	raw, err := prompts.Read("fileEditor.md")
	if err != nil {
		logger.Warn(Module, "[文件编辑] 读取系统提示词失败: %v", err)
		return "你是「月华.文件编辑」智能体，负责维护 /document 文档库中的文件。"
	}
	return string(raw)
}

// dispatchTool 按工具名分派（自然语言模式使用）。
func dispatchTool(ctx context.Context, name string, args map[string]any) OpResult {
	// 压缩包 / GGUF 属二进制容器：普通文件工具一律拒绝，引导走 task_scheduling
	if isFileTool(name) {
		if path := argString(args, "path"); isArchivePath(path) {
			return failResult(name, path, fmt.Errorf(
				"压缩包、GGUF 等二进制容器请用 %s（type=archive_ops / archive_reader）调度处理", ToolSchedule))
		}
	}
	switch name {
	case ToolList:
		return FileList(argString(args, "path"))
	case ToolRead:
		return ReadFile(argString(args, "path"), argInt(args, "offset", 0), argInt(args, "max_chars", 0))
	case ToolWrite:
		return WriteFile(argString(args, "path"), argString(args, "content"))
	case ToolCreate:
		return CreateFile(argString(args, "path"), argString(args, "content"))
	case ToolDelete:
		return DeleteFile(argString(args, "path"))
	case ToolCopy:
		return CopyFile(argString(args, "path"), argString(args, "target"))
	case ToolMove:
		return MoveFile(argString(args, "path"), argString(args, "target"))
	case ToolRename:
		return RenameFile(argString(args, "path"), argString(args, "name"))
	case ToolSearch:
		return SearchContent(argString(args, "path"), argString(args, "query"))
	case ToolReplace:
		return ReplaceAt(argString(args, "path"),
			argInt(args, "start_line", 1), argInt(args, "end_line", argInt(args, "start_line", 1)),
			argString(args, "content"))
	case ToolInfo:
		return FileAttr(argString(args, "path"))
	case ToolWeb:
		return WebQuery(ctx, argString(args, "query"))
	case ToolRunProj:
		return RunProjectWithLimit(ctx, argString(args, "path"), argInt(args, "timeout_sec", 0))
	case ToolSchedule:
		return scheduleTask(ctx, args)
	default:
		return failResult(name, "", errUnknownTool)
	}
}

// ==== 参数解析 ====

func argString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func argInt(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}

// validKindCode 判断句柄类型码是否合法（D/I/G/A/V/U）。
func validKindCode(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "D", "I", "G", "A", "V", "U":
		return true
	}
	return false
}

var (
	errEmptyInstruction = errorText("请用自然语言说明本次文件编辑要做什么")
	errUnknownTool      = errorText("未知工具")
)

// errorText 轻量错误类型（免引入额外错误包装）。
type errorText string

func (e errorText) Error() string { return string(e) }
