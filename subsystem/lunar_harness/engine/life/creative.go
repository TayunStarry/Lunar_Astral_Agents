package life

// creative.go 生活创作参考字符串：由 life_planning 写入，生活创作读取，
// 一轮创作结束后按值条件重置，避免同一件已完成的事被重复执行。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// CreativeIntent 读取当前创作参考字符串；没有时返回空串。
func CreativeIntent() string {
	creativeMu.Lock()
	defer creativeMu.Unlock()
	raw, ok := persistence.ReadFileBytes(creativeIntentFile)
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SetCreativeIntent 用对话中给出的描述覆盖创作参考字符串。
func SetCreativeIntent(text string) error {
	text = strings.TrimSpace(text)
	creativeMu.Lock()
	defer creativeMu.Unlock()
	if err := persistence.WriteFileAtomic(creativeIntentFile, []byte(text)); err != nil {
		logger.Warn(Module, "[创作参考] 写入失败: %v", err)
		return err
	}
	logger.Info(Module, "[创作参考] 已更新：%s", truncateRunes(text, 80))
	return nil
}

// ResetCreativeIntentIf 仅当当前值与 expected 一致时清空，避免覆盖运行期间新写入的意图。
func ResetCreativeIntentIf(expected string) {
	expected = strings.TrimSpace(expected)
	creativeMu.Lock()
	defer creativeMu.Unlock()
	raw, ok := persistence.ReadFileBytes(creativeIntentFile)
	if !ok {
		return
	}
	if strings.TrimSpace(string(raw)) != expected {
		return
	}
	if err := persistence.WriteFileAtomic(creativeIntentFile, []byte("")); err != nil {
		logger.Warn(Module, "[创作参考] 重置失败: %v", err)
		return
	}
	logger.Info(Module, "[创作参考] 本轮已完成，参考字符串已重置")
}

// ResetCreativeIntent 无条件清空创作参考字符串（供测试与维护入口使用）。
func ResetCreativeIntent() error {
	creativeMu.Lock()
	defer creativeMu.Unlock()
	return persistence.WriteFileAtomic(creativeIntentFile, []byte(""))
}
