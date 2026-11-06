package viewer

// notify.go 被动消息的场景、提示材料与日志辅助（回应链路见 reply_delivery.go）。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ViewerCritic"
	"LunarSubsystem/LunarHarness/prompts"
)

// inboxBrief 通知条目的关键字段摘要（失败日志排查用）
func inboxBrief(item inboxItem) string {
	return fmt.Sprintf("type=%d oid=%s src=%s root=%s target=%s uri=%s",
		item.Type, item.Oid, item.SourceID, item.RootID, item.TargetID, item.URI)
}

// generateInboxReply 按场景生成一句自然回应（注入人设与长期记忆）。
func generateInboxReply(item inboxItem) (string, error) {
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", fmt.Errorf("读取人设失败: %w", err)
	}
	scene := inboxScene(item)
	var material strings.Builder
	fmt.Fprintf(&material, "对方：%s\n对方的消息：%s\n", item.User, item.Message)
	if item.MyContent != "" {
		fmt.Fprintf(&material, "我原来发的内容：%s\n", item.MyContent)
	}
	if item.Title != "" {
		fmt.Fprintf(&material, "所在位置：《%s》\n", item.Title)
	}
	text, err := ViewerCritic.InboxReply(context.Background(), ViewerCritic.InboxInput{
		Persona: string(persona), Scene: scene, Memory: memoryMaterial(10),
		Material: material.String(), Model: modelPrimary,
	})
	if err != nil {
		return "", err
	}
	return finishComment(text, dynamicCommentMaxRunes), nil
}

// inboxScene 互动发生的场景描述。
func inboxScene(item inboxItem) string {
	venue := "视频评论区"
	switch {
	case item.Venue == "dynamic" || item.Type == 17 || item.Type == 11:
		venue = "动态区"
	case item.Venue == "article" || item.Type == 12:
		venue = "专栏评论区"
	}
	if item.Title != "" {
		return "《" + item.Title + "》的" + venue
	}
	return venue
}
