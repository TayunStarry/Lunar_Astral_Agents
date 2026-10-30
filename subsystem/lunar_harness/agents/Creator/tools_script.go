package Creator

// tools_script.go 代码运行类工具：统一委托「文件编辑」智能体的运行入口。
//
// 运行分流由智能体实现：文件头含「//Engine: Lunar Harness」走月华私有 js 运行时，
// 其余 js/html 走浏览器运行环境（纯 js 自动生成可展示结果的模板网页容器）。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
)

func runScriptTool(args map[string]any) toolOutcome {
	scripts := argStrings(args, "scripts")
	if len(scripts) == 0 {
		return failure(toolRunScript, fmt.Errorf("scripts 不能为空"))
	}
	results := make([]FileEditor.OpResult, 0, len(scripts))
	summaries := make([]string, 0, len(scripts))
	failed := 0
	timeoutSec := argInt(args, "timeout_sec", 0)
	for _, rel := range scripts {
		results = append(results, FileEditor.RunProjectPathWithLimit(rel, timeoutSec))
		if !results[len(results)-1].OK {
			failed++
		}
		summaries = append(summaries, results[len(results)-1].Summary)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"results": results}),
		Summary: fmt.Sprintf("运行 %d 个脚本（失败 %d）：%v", len(scripts), failed, summaries),
	}
}

func scriptStatusTool(args map[string]any) toolOutcome {
	_ = args
	return toolOutcome{
		JSON:    jsonText(map[string]any{"loaded": false, "note": "运行结果由文件编辑智能体直接返回，不再维护独立运行状态"}),
		Summary: "脚本状态：由文件编辑智能体即时返回",
	}
}
