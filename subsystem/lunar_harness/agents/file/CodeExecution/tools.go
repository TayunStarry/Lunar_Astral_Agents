package CodeExecution

// tools.go 代码执行工具：运行 HTML/JavaScript 与解析函数、依赖路径。

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/agents/file/FileEditor"
	"LunarSubsystem/LunarHarness/agents/file/filesupport"
	"LunarSubsystem/LunarHarness/engine/resource"
)

const (
	toolExecute = "execute_code"
	toolParse   = "parse_code"
)

var (
	reFunction  = regexp.MustCompile(`^(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(([^)]*)\)`)
	reArrowFunc = regexp.MustCompile(`^(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:function\s*)?\(([^)]*)\)\s*=>`)
	reFuncExpr  = regexp.MustCompile(`^(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?function\s*\(([^)]*)\)`)
	reImport    = regexp.MustCompile(`(?m)^\s*import\s+.*?from\s+['"]([^'"]+)['"]`)
	reRequire   = regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\s*\)`)
	reScriptSrc = regexp.MustCompile(`(?i)<script[^>]+src\s*=\s*["']([^"']+)["']`)
	reLinkHref  = regexp.MustCompile(`(?i)<link[^>]+href\s*=\s*["']([^"']+)["']`)
)

func toolHandler(c *collector) file.Handler {
	return func(name string, args map[string]any) (file.Outcome, bool, error) {
		switch name {
		case toolExecute:
			return execute(context.Background(), args, c), false, nil
		case toolParse:
			return parse(args, c), false, nil
		default:
			return file.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func execute(ctx context.Context, args map[string]any, c *collector) file.Outcome {
	rel, _, err := filesupport.Resolve(file.StringArg(args, "path"), c.root)
	if err != nil {
		return file.Failure(toolExecute, err)
	}
	timeout := file.IntArg(args, "timeout_sec")
	result := FileEditor.RunProjectWithLimit(ctx, rel, timeout)
	c.add(result.Handles...)
	return file.Outcome{JSON: result.JSON(), Summary: result.Summary, Handles: result.Handles}
}

func parse(args map[string]any, c *collector) file.Outcome {
	rel, _, err := filesupport.Resolve(file.StringArg(args, "path"), c.root)
	if err != nil {
		return file.Failure(toolParse, err)
	}
	source, err := FileEditor.ReadText(rel)
	if err != nil {
		return file.Failure(toolParse, err)
	}
	functions := make([]string, 0, 8)
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		body := strings.TrimPrefix(line, "export ")
		body = strings.TrimPrefix(body, "default ")
		if match := reFunction.FindStringSubmatch(body); match != nil {
			functions = append(functions, match[1]+"("+compact(match[2])+")")
			continue
		}
		if match := reArrowFunc.FindStringSubmatch(body); match != nil {
			functions = append(functions, match[1]+"("+compact(match[2])+") =>")
			continue
		}
		if match := reFuncExpr.FindStringSubmatch(body); match != nil {
			functions = append(functions, match[1]+"("+compact(match[2])+")")
		}
	}
	references := make([]string, 0, 8)
	for _, pattern := range []*regexp.Regexp{reImport, reRequire, reScriptSrc, reLinkHref} {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			if len(match) == 2 && !contains(references, match[1]) {
				references = append(references, match[1])
			}
		}
	}
	c.add()
	return file.Outcome{
		JSON: file.JSON(map[string]any{
			"ok": true, "path": rel, "functions": functions, "references": references,
		}),
		Summary: fmt.Sprintf("%s：%d 个函数，%d 个引用", rel, len(functions), len(references)),
	}
}

func compact(params string) string {
	return strings.Join(strings.Fields(params), " ")
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
