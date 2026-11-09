package thinking

// runtime_message.go 对话者运行时消息：全局共享的时间/心情/情感标识/刚做过的事。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/engine/activity"
)

func currentGlobalMessage() RuntimeGlobalMessage {
	moodSnapshot := wishchain.SnapshotMood()
	return RuntimeGlobalMessage{
		Timestamp:    time.Now().Unix(),
		MoodLabel:    moodSnapshot.Label,
		EmotionMarks: emotionMarks(),
	}
}

func buildRuntimeGlobalMessage() PostMessage {
	message := currentGlobalMessage()
	// 活动描述每轮重新读取，业务链条 Set 后下一轮即可见。
	text := fmt.Sprintf(
		"【当前状态】\n当前时间：%s\n当前心情：%s\n%s\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		emptyAsText(message.MoodLabel, "开心"),
		activity.Line(),
		wishchain.FormatList(),
	)
	if len(message.EmotionMarks) > 0 {
		text += "\n" + strings.Join(message.EmotionMarks, "\n")
	}
	return PostMessage{Role: RoleUser, Content: text}
}

func emptyAsText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
