package LifeMemorizer

// text.go 句读截断的本地入口；具体实现统一在 internal/textutil。

import "LunarSubsystem/LunarHarness/internal/textutil"

// truncateAtSentence 在句读处切断，避免把一句话截成半句。
// 使用 MinFill 变体：句读位置过早（保留不足 60% 预算）时宁可硬切，见 textutil.TruncateAtSentenceMinFill。
func truncateAtSentence(text string, limit int) string {
	return textutil.TruncateAtSentenceMinFill(text, limit)
}
