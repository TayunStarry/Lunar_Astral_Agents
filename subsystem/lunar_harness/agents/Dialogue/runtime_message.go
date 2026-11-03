package Dialogue

// runtime_message.go 对话者运行时消息：全局共享的时间/心情/情感标识。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/mood"
)

func currentGlobalMessage() RuntimeGlobalMessage {
	moodSnapshot := mood.Snapshot()
	return RuntimeGlobalMessage{
		Timestamp:    time.Now().Unix(),
		MoodLabel:    moodSnapshot.Label,
		EmotionMarks: emotionMarks(),
	}
}

func buildRuntimeGlobalMessage() PostMessage {
	message := currentGlobalMessage()
	text := fmt.Sprintf(
		"【当前状态】\n当前时间：%s\n当前心情：%s",
		time.Now().Format("2006-01-02 15:04:05"),
		emptyAsText(message.MoodLabel, "开心"),
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
