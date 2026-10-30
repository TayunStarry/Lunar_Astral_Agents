package FileEditor

// agent.go 文件编辑智能体对外入口：自然语言指令 → 工具循环 → 中文简述。
//
// 除自然语言入口外，本包还导出逐项能力（ReadFile / WriteFile / …），
// 供「月华-生活系统-创作」在同一进程内直接调度，返回值同样带句柄与操作简述。

import (
	"context"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
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
		return "", err
	}
	if summary == "" {
		summary = "已完成文件编辑操作"
	}
	logger.Info(Module, "[文件编辑] 本轮完成：%s", previewRunes(summary, 120))
	return summary, nil
}

// Execute 用自然语言执行一次文件编辑，返回结构化结果（含资源句柄与操作简述）。
// 供「月华-对话者」（dispatch_technique → file_editor）等进程内调用方使用。
func Execute(ctx context.Context, instruction string) (OpResult, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return failResult("file_editor", "", errEmptyInstruction), errEmptyInstruction
	}
	runMu.Lock()
	defer runMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	summary, handles, err := runToolLoop(runCtx, systemPrompt(), instruction)
	if err != nil {
		logger.Warn(Module, "[文件编辑] 执行失败: %v", err)
		return failResult("file_editor", "", err), err
	}
	if summary == "" {
		summary = "已完成文件编辑操作"
	}
	logger.Info(Module, "[文件编辑] 本轮完成：%s", previewRunes(summary, 120))
	return newResult("file_editor", "", summary, handles, nil), nil
}

// systemPrompt 文件编辑智能体的系统提示。
func systemPrompt() string {
	return `你是「月华.文件编辑」智能体，负责维护 /document 文档库中的文件。

工作原则：
1. 修改已有文件前必须先 file_read 读出当前内容，禁止凭印象整篇覆盖。
2. 小范围改动优先用 file_replace_at（按行置换），只有大改时才用 file_write 整篇重写。
3. 需要外部资料时可用 web_search / memory_search；查找已登记资源用 handle_search / handle_read。
4. 运行 js/html 项目文件用 run_project：文件头含「//Engine: Lunar Harness」的走月华私有 js 运行时，
   其余走浏览器运行环境（纯 js 会自动套模板网页展示输出）。
5. 运行时长默认上限 120 秒；需要更久时在调用 run_project 时传 timeout_sec（5-3600 秒），
   或在文件第一行写明「//Engine: Lunar Harness | timeout=300s」。订阅型脚本到期会被自动卸载。
6. 所有文件路径一律写相对 /document 的路径。
7. 每完成一步都用 1-2 句话说明「对哪个文件做了什么」，不要贴大段代码；不要臆造工具结果。`
}

// dispatchTool 按工具名分派（自然语言模式使用）。
func dispatchTool(ctx context.Context, name string, args map[string]any) OpResult {
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
	case ToolMemory:
		return MemoryQuery(ctx, argString(args, "query"), argInt(args, "limit", 8))
	case ToolHandleSr:
		return HandleSearch(argString(args, "query"), argInt(args, "limit", 8))
	case ToolHandleRd:
		return HandleRead(argString(args, "handle"), argInt(args, "max_bytes", 0))
	case ToolRunProj:
		return RunProjectWithLimit(ctx, argString(args, "path"), argInt(args, "timeout_sec", 0))
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

var (
	errEmptyInstruction = errorText("请用自然语言说明本次文件编辑要做什么")
	errUnknownTool      = errorText("未知工具")
)

// errorText 轻量错误类型（免引入额外错误包装）。
type errorText string

func (e errorText) Error() string { return string(e) }
