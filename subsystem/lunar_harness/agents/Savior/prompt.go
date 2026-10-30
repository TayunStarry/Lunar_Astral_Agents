package Savior

// prompt.go 拯救者系统提示词：基础提示词 + 本次故障 + 当前可修复目标。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/agentrepair"
	"LunarSubsystem/LunarHarness/prompts"
)

// buildSystemPrompt 组装本次修复的系统提示词。
func buildSystemPrompt(failure *Failure) (string, error) {
	data, err := prompts.Read("savior.md")
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(string(data)))
	builder.WriteString("\n\n## 本次故障\n\n")
	fmt.Fprintf(&builder, "- 故障编号：%d\n", failure.ID)
	fmt.Fprintf(&builder, "- 出错智能体：%s\n", failure.Agent)
	fmt.Fprintf(&builder, "- 错误信息：%s\n", previewRunes(failure.Error, 600))
	fmt.Fprintf(&builder, "- 发生时间：%s\n", failure.At.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&builder, "- 已尝试次数：%d\n", failure.Attempts)
	builder.WriteString("\n## 可修复目标\n\n")
	targets := agentrepair.List()
	if len(targets) == 0 {
		builder.WriteString("- 暂无注册的可修复目标：只能读取上下文，然后如实说明无法自动修复。\n")
	}
	for _, info := range targets {
		fmt.Fprintf(&builder, "- %s（%s）：%s；可用动作：%s\n",
			info.ID, info.Label, info.Description, strings.Join(info.Actions, " / "))
	}
	return builder.String(), nil
}
