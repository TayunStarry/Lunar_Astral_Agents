package FileOperations

// agent.go 文件操作智能体：拆解任务、派发文件 / 归档 / 代码 / 回滚子智能体，
// 按实际执行结果返回；失败时先让创作系统自行修复，重试仍失败才回滚本次修改。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/filesupport"
	"LunarSubsystem/LunarHarness/agents/toolagent"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次文件操作任务；失败时最多重试 maxRepairRetries 次，仍未成功才回滚。
func Run(ctx context.Context, req Request) (Result, error) {
	collector := &collector{root: req.WorkspaceRoot, startedUnix: time.Now().Unix()}
	baseInstruction := withHandlePaths(req.Instruction, mergeHandles(req.Handles, handlesFromText(req.Instruction)))
	instruction := baseInstruction
	var lastErr error
	for attempt := 0; attempt <= maxRepairRetries; attempt++ {
		if attempt > 0 {
			collector.dropFailedSteps()
			instruction = repairInstruction(baseInstruction, lastErr, collector)
		}
		summary, err := toolagent.Run(ctx, toolagent.Request{
			AgentID:          AgentID,
			Role:             Role,
			SystemPrompt:     prompts.ReadOr(promptFile, "你是文件操作智能体，负责拆解任务并派发子智能体。"),
			Instruction:      instruction,
			Tools:            definitions(),
			Handler:          toolHandler(ctx, collector, req),
			MaxRounds:        maxRounds,
			MaxTokens:        4096,
			SummaryMaxTokens: 320,
		})
		if collector.hasFailure() {
			err = fmt.Errorf("%s", collector.failureText())
		}
		if err == nil && !collector.hasOperations() {
			err = fmt.Errorf("没有执行任何文件操纵操作")
		}
		if err == nil {
			return successResult(summary, collector), nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	rollbackNote := ""
	if err := rollbackPaths(collector.changed(), collector.startedUnix); err != nil {
		rollbackNote = "；自动回滚失败：" + err.Error()
	} else if len(collector.changed()) > 0 {
		rollbackNote = "；已回退本次全部修改"
	}
	return failedResult(collector), fmt.Errorf("%v%s", lastErr, rollbackNote)
}

// successResult 汇总成功结果。
func successResult(summary string, c *collector) Result {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = strings.TrimSpace(c.operationsText())
	}
	if summary == "" {
		summary = "文件操作已完成"
	}
	return Result{
		Summary: summary,
		Handles: c.handles(),
		Changed: c.changed(),
		Folder:  projectFolder(c.changed()),
	}
}

// failedResult 汇总失败结果，保留已产生的句柄与变更供上层判断。
func failedResult(c *collector) Result {
	summary := strings.TrimSpace(c.operationsText())
	if summary == "" {
		summary = "文件操作失败"
	}
	return Result{
		Summary: summary,
		Handles: c.handles(),
		Changed: c.changed(),
		Folder:  projectFolder(c.changed()),
	}
}

// repairInstruction 把失败原因与已改文件交给创作系统，要求只修未完成的部分。
func repairInstruction(base string, lastErr error, c *collector) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n【上一次尝试失败】\n")
	b.WriteString(strings.TrimSpace(lastErr.Error()))
	if changed := c.changed(); len(changed) > 0 {
		b.WriteString("\n已修改的文件（不要重复写入，先读取确认）：")
		b.WriteString(strings.Join(changed, "、"))
	}
	b.WriteString("\n请先检查现有文件，只修复未完成或出错的部分，不要从头重做，也不要删除已经写好的内容。")
	return b.String()
}

// projectFolder 从变更文件路径推断项目目录（documents/<标题>）；
// 文件分散在多个项目目录或直接写在 documents/ 下时返回空串。
func projectFolder(paths []string) string {
	folder := ""
	for _, rel := range paths {
		rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
		rest, ok := strings.CutPrefix(rel, "documents/")
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(rest, "/")
		if !ok || name == "" {
			continue
		}
		current := "documents/" + name
		if folder == "" {
			folder = current
			continue
		}
		if folder != current {
			return ""
		}
	}
	return folder
}

func rollbackPaths(paths []string, sinceUnix int64) error {
	var failures []string
	for _, rel := range paths {
		version, ok := filesupport.OldestSince(rel, sinceUnix)
		if !ok {
			version, ok = filesupport.Latest(rel)
		}
		if !ok {
			continue
		}
		if err := filesupport.Restore(rel, version.ID); err != nil {
			failures = append(failures, rel+"："+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "；"))
	}
	return nil
}

func withHandlePaths(instruction string, handles []string) string {
	lines := make([]string, 0, len(handles))
	for _, handle := range handles {
		entry, err := resolveHandle(handle)
		if err == nil && strings.TrimSpace(entry) != "" {
			lines = append(lines, "- "+handle+" → "+entry)
		} else {
			lines = append(lines, "- "+handle)
		}
	}
	if len(lines) == 0 {
		return instruction
	}
	return instruction + "\n\n【目标句柄】\n" + strings.Join(lines, "\n")
}
