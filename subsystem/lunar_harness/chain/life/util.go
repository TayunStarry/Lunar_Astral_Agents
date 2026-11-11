package life

import (
	"strings"

	"LunarSubsystem/LunarHarness/internal/textutil"
)

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// truncateAtSentence 在句读处切断，避免把一句话截成半句；实现统一在 internal/textutil。
func truncateAtSentence(text string, limit int) string {
	return textutil.TruncateAtSentence(text, limit)
}
