package MemoryTagger

// messages.go 消息构造：把请求整理成文本 / 单图 / 动态图帧序列三种上下文。

import "strings"

// buildMessages 按请求形态构造消息数组。
func buildMessages(req Request) []chatMessage {
	if len(req.MediaRefs) > 0 {
		return mediaMessages(req)
	}
	if req.IsImage {
		return imageMessages(req)
	}
	return textMessages(req)
}

// textMessages 文本标签请求。
func textMessages(req Request) []chatMessage {
	return []chatMessage{
		{Role: "system", Content: textSystemPrompt},
		{Role: "user", Content: "请为以下文本生成标签，以JSON数组格式返回：\n\n" + req.Content},
	}
}

// imageMessages 单张图片标签请求。
func imageMessages(req Request) []chatMessage {
	orientation := normalizeOrientation(req.Orientation, req.Custom)
	return []chatMessage{
		{Role: "system", Content: imageSystemPrompt(orientation, req.Custom)},
		{Role: "user", Content: []chatContentPart{
			{Type: "text", Text: "请为这张图片生成标签，以JSON数组格式返回"},
			{Type: "image_url", ImageURL: &imageURL{URL: req.Content}},
		}},
	}
}

// mediaMessages 动态图采样帧序列标签请求。
func mediaMessages(req Request) []chatMessage {
	orientation := normalizeOrientation(req.Orientation, req.Custom)
	parts := make([]chatContentPart, 0, len(req.MediaRefs)+1)
	parts = append(parts, chatContentPart{
		Type: "text",
		Text: "以下是同一段动画按时间顺序抽取的采样帧，请综合动作过程与画面内容生成标签，以JSON数组格式返回",
	})
	for _, ref := range req.MediaRefs {
		parts = append(parts, chatContentPart{Type: "image_url", ImageURL: &imageURL{URL: ref}})
	}
	return []chatMessage{
		{Role: "system", Content: imageSystemPrompt(orientation, req.Custom) + mediaFrameNote},
		{Role: "user", Content: parts},
	}
}

// truncate 截断字符串用于错误输出。
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return strings.TrimSpace(string(runes[:maxLen])) + "..."
}
