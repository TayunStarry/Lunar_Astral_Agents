package Creator

// tools_script.go 代码运行类工具：LTP9 脚本运行与状态监控。

import "fmt"

func runScriptTool(args map[string]any) toolOutcome {
	scripts := argStrings(args, "scripts")
	result, err := runLTP9Scripts(scripts)
	if err != nil {
		return failure(toolRunScript, err)
	}
	summary := "运行 LTP9 脚本"
	if result.Error != "" {
		summary += "失败：" + previewRunes(result.Error, 80)
	} else {
		summary += "：已加载"
	}
	return toolOutcome{
		JSON:    jsonText(result),
		Summary: summary,
	}
}

func scriptStatusTool(args map[string]any) toolOutcome {
	result, err := scriptStatus(argString(args, "name"))
	if err != nil {
		return failure(toolScriptStatus, err)
	}
	return toolOutcome{
		JSON:    jsonText(result),
		Summary: fmt.Sprintf("脚本状态：loaded=%v error=%s", result.Loaded, previewRunes(result.Error, 80)),
	}
}
