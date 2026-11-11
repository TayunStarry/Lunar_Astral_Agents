package FileOperations

// tools.go 文件操作工具：资源读取与批量操纵派发。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/agents/file/CodeExecution"
	"LunarSubsystem/LunarHarness/agents/file/ContentRollback"
	"LunarSubsystem/LunarHarness/agents/file/ManipulateArchive"
	"LunarSubsystem/LunarHarness/agents/file/ManipulateFiles"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

const (
	toolFetch      = "fetch_resources"
	toolManipulate = "execute_manipulation"

	maxSteps = 6
)

type stepResult struct {
	Type    string   `json:"type"`
	OK      bool     `json:"ok"`
	Summary string   `json:"summary"`
	Handles []string `json:"handles,omitempty"`
	Changed []string `json:"changed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type collector struct {
	root         string
	startedUnix  int64
	handle       []string
	changedPaths []string
	steps        []stepResult
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

func (c *collector) addChanged(paths ...string) {
	for _, rel := range paths {
		rel = strings.TrimSpace(rel)
		if rel == "" || contains(c.changedPaths, rel) {
			continue
		}
		c.changedPaths = append(c.changedPaths, rel)
	}
}

func (c *collector) handles() []string { return append([]string(nil), c.handle...) }
func (c *collector) changed() []string { return append([]string(nil), c.changedPaths...) }

// dropFailedSteps 重试前丢弃上一轮的失败步骤，保留已成功的步骤、句柄与变更。
func (c *collector) dropFailedSteps() {
	steps := make([]stepResult, 0, len(c.steps))
	for _, step := range c.steps {
		if step.Error == "" {
			steps = append(steps, step)
		}
	}
	c.steps = steps
}

func (c *collector) hasOperations() bool { return len(c.steps) > 0 }

// hasFailure 是否有步骤以失败结束。
func (c *collector) hasFailure() bool {
	for _, step := range c.steps {
		if step.Error != "" {
			return true
		}
	}
	return false
}

// failureText 返回第一个失败步骤的可读原因。
func (c *collector) failureText() string {
	for _, step := range c.steps {
		if step.Error != "" {
			return step.Error
		}
	}
	return "文件操作失败"
}

func (c *collector) operationsText() string {
	var b strings.Builder
	for index, step := range c.steps {
		b.WriteString(fmt.Sprintf("%d. [%s] ", index+1, step.Type))
		if step.OK {
			b.WriteString("成功：")
		} else {
			b.WriteString("失败：")
		}
		b.WriteString(step.Summary)
		if step.Error != "" {
			b.WriteString("；错误：")
			b.WriteString(step.Error)
		}
		if len(step.Changed) > 0 {
			b.WriteString("；变更：")
			b.WriteString(strings.Join(step.Changed, "、"))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func toolHandler(ctx context.Context, c *collector, req Request) file.Handler {
	return func(name string, args map[string]any) (file.Outcome, bool, error) {
		switch name {
		case toolFetch:
			result := resourcehub.RunArgs(context.Background(), args)
			return file.Outcome{JSON: result.Text, Summary: "已读取资源"}, false, nil
		case toolManipulate:
			outcome := manipulate(ctx, args, c, req)
			// 任一步失败立即结束外层循环，避免模型重复提交同样的失败操作。
			return outcome, c.hasFailure(), nil
		default:
			return file.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func manipulate(ctx context.Context, args map[string]any, c *collector, req Request) file.Outcome {
	raw, ok := args["operations"].([]any)
	if !ok || len(raw) == 0 {
		return file.Failure(toolManipulate, fmt.Errorf("operations 不能为空"))
	}
	if len(raw) > maxSteps {
		return file.Failure(toolManipulate, fmt.Errorf("单次最多执行 %d 个步骤", maxSteps))
	}
	for index, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			return failSteps(c, fmt.Sprintf("第 %d 步格式不正确", index+1))
		}
		kind := strings.ToLower(file.StringArg(fields, "type"))
		description := file.StringArg(fields, "description")
		if description == "" {
			return failSteps(c, fmt.Sprintf("第 %d 步缺少 description", index+1))
		}
		step, err := dispatch(ctx, kind, description, req)
		step.OK = err == nil
		if err != nil && len(step.Changed) > 0 && errors.Is(err, context.DeadlineExceeded) {
			// 模型总结超时，但文件修改已经落盘：按成功处理，避免重复写入。
			step.OK = true
			if strings.TrimSpace(step.Summary) == "" {
				step.Summary = "文件已修改"
			}
			step.Summary += "（模型总结超时，但写入已完成）"
			err = nil
		}
		c.steps = append(c.steps, step)
		c.addHandle(step.Handles...)
		c.addChanged(step.Changed...)
		if err != nil {
			// 不立即回滚：由 Run 先让创作系统修复，重试仍失败才统一回退。
			step.OK = false
			step.Error = err.Error()
			c.steps[len(c.steps)-1] = step
			return failStepsWith(c, step.Error)
		}
	}
	outcome := file.Outcome{
		JSON:    file.JSON(map[string]any{"ok": true, "steps": c.steps}),
		Summary: c.operationsText(),
	}
	if len(c.changed()) > 0 {
		outcome.Mutated = true
	}
	return outcome
}

func dispatch(ctx context.Context, kind, description string, req Request) (stepResult, error) {
	step := stepResult{Type: kind}
	handles := mergeHandles(req.Handles, handlesFromText(description))
	switch kind {
	case "manipulate_files":
		result, err := ManipulateFiles.Run(ctx, ManipulateFiles.Request{
			Instruction: description, Handles: handles, WorkspaceRoot: req.WorkspaceRoot,
		})
		step.Summary, step.Handles, step.Changed = result.Summary, result.Handles, result.Changed
		return step, err
	case "manipulate_archive":
		result, err := ManipulateArchive.Run(ctx, ManipulateArchive.Request{
			Instruction: description, Handles: handles, WorkspaceRoot: req.WorkspaceRoot,
		})
		step.Summary, step.Handles, step.Changed = result.Summary, result.Handles, result.Changed
		return step, err
	case "execute_code":
		result, err := CodeExecution.Run(ctx, CodeExecution.Request{
			Instruction: description, Handles: handles, WorkspaceRoot: req.WorkspaceRoot,
		})
		step.Summary, step.Handles = result.Summary, result.Handles
		return step, err
	case "rollback":
		result, err := ContentRollback.Run(ctx, ContentRollback.Request{
			Instruction: description, Handles: handles, WorkspaceRoot: req.WorkspaceRoot,
		})
		step.Summary, step.Handles, step.Changed = result.Summary, result.Handles, result.Changed
		return step, err
	default:
		step.Summary = "未知操作类型：" + kind
		return step, fmt.Errorf("type 只能取 manipulate_files / manipulate_archive / execute_code / rollback")
	}
}

func failSteps(c *collector, message string) file.Outcome {
	step := stepResult{Type: "invalid", Summary: message, Error: message}
	c.steps = append(c.steps, step)
	return failStepsWith(c, message)
}

func failStepsWith(c *collector, message string) file.Outcome {
	return file.Outcome{
		JSON:    file.JSON(map[string]any{"ok": false, "steps": c.steps, "error": message}),
		Summary: message,
	}
}

func resolveHandle(handle string) (string, error) {
	entry, err := resource.Resolve(handle)
	if err != nil {
		return "", err
	}
	return entry.BlobPath, nil
}

// handlePattern 资源句柄文法（与 engine/resource 保持一致）。
var handlePattern = regexp.MustCompile(`@[DIVAGTU]\.\d{3,}`)

// handlesFromText 从自然语言里提取资源句柄，保证子智能体拿到明确目标。
func handlesFromText(text string) []string {
	return dedupeHandles(handlePattern.FindAllString(text, -1))
}

// mergeHandles 合并显式句柄与文本中出现的句柄。
func mergeHandles(explicit []string, extra []string) []string {
	return dedupeHandles(append(append([]string(nil), explicit...), extra...))
}

func dedupeHandles(handles []string) []string {
	out := make([]string, 0, len(handles))
	seen := map[string]bool{}
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || seen[handle] {
			continue
		}
		seen[handle] = true
		out = append(out, handle)
	}
	return out
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}
