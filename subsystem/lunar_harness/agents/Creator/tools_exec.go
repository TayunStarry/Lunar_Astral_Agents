package Creator

// tools_exec.go 工具分发入口与参数解析。
// 具体能力按职责拆到 tools_search / tools_files / tools_dispatch / tools_script。

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// executeTool 执行一个工具调用。
func (s *session) executeTool(ctx context.Context, name string, args map[string]any) toolOutcome {
	switch name {
	case toolWebSearch:
		return s.webSearch(args)
	case toolMemorySearch:
		return s.memorySearch(ctx, args)
	case toolHandleSearch:
		return handleSearch(args)
	case toolHandleRead:
		return handleRead(args)
	case toolFileList:
		return fileList(args)
	case toolFileRead:
		return fileRead(args)
	case toolFileCreate:
		return fileCreate(args)
	case toolFileRewrite:
		return fileRewrite(args)
	case toolFileRemove:
		return fileRemove(args)
	case toolFileDelete:
		return fileDelete(args)
	case toolFileMove:
		return fileMove(args)
	case toolFileReplace:
		return fileReplace(args)
	case toolFileDeleteText:
		return fileDeleteText(args)
	case toolCodeIndex:
		return codeIndexTool(args)
	case toolReadDTS:
		return readDTSTool(args)
	case toolSendGroupText:
		return sendGroupText(args)
	case toolSendGroupFile:
		return sendGroupFile(args)
	case toolRunScript:
		return runScriptTool(args)
	case toolScriptStatus:
		return scriptStatusTool(args)
	default:
		return failure(name, fmt.Errorf("未知工具"))
	}
}

// failure 构造失败回填。
func failure(name string, err error) toolOutcome {
	message := name + " 失败：" + err.Error()
	return toolOutcome{
		JSON:    jsonText(map[string]any{"error": err.Error()}),
		Summary: previewRunes(message, 200),
	}
}

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

func argBool(args map[string]any, key string) bool {
	switch value := args[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	}
	return false
}

func argStrings(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		if single, ok := args[key].(string); ok && strings.TrimSpace(single) != "" {
			return []string{single}
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}
