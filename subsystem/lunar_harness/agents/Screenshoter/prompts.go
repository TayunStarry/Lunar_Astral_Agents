package Screenshoter

// prompts.go 截图者提示词读取。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// systemPrompt 读取截图者系统提示词。
func systemPrompt() string {
	raw, err := prompts.Read(promptPath)
	if err != nil {
		logger.Error(module, "读取提示词 %s 失败: %v", promptPath, err)
		return ""
	}
	return string(raw)
}
