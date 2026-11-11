package ContentRollback

// tools.go 回滚工具：列出历史版本与回滚到指定版本。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/filesupport"
	"LunarSubsystem/LunarHarness/agents/toolagent"
	"LunarSubsystem/LunarHarness/engine/resource"
)

const (
	toolList     = "rollback_list"
	toolRollback = "rollback"
)

func toolHandler(c *collector) toolagent.Handler {
	return func(name string, args map[string]any) (toolagent.Outcome, bool, error) {
		switch name {
		case toolList:
			return listVersions(args, c), false, nil
		case toolRollback:
			return rollback(args, c), false, nil
		default:
			return toolagent.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func listVersions(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolList, err)
	}
	versions, err := filesupport.List(rel)
	if err != nil {
		return toolagent.Failure(toolList, err)
	}
	return toolagent.Outcome{
		JSON:    toolagent.JSON(map[string]any{"ok": true, "path": rel, "versions": versions}),
		Summary: fmt.Sprintf("%s 共 %d 个历史版本", rel, len(versions)),
	}
}

func rollback(args map[string]any, c *collector) toolagent.Outcome {
	rel, _, err := filesupport.Resolve(toolagent.StringArg(args, "path"), c.root)
	if err != nil {
		return toolagent.Failure(toolRollback, err)
	}
	versionID := toolagent.StringArg(args, "version")
	if versionID == "" {
		latest, ok := filesupport.Latest(rel)
		if !ok {
			return toolagent.Failure(toolRollback, fmt.Errorf("没有可回滚的版本"))
		}
		versionID = latest.ID
	}
	if err := filesupport.Restore(rel, versionID); err != nil {
		return toolagent.Failure(toolRollback, err)
	}
	c.addChanged(rel)
	handle := ""
	if _, registered, regErr := resource.RegisterPath(rel); regErr == nil {
		handle = registered
		c.addHandle(handle)
	}
	return toolagent.Outcome{
		JSON:     toolagent.JSON(map[string]any{"ok": true, "path": rel, "version": versionID, "handle": handle}),
		Summary:  "已回滚 " + rel + " 到版本 " + versionID,
		Mutated:  true,
		Evidence: "回滚 " + rel + " 到版本 " + versionID,
	}
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
