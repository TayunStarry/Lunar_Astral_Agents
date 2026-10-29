package lunar

// user_text.go 记录最新用户发言与发送者，供生活体系更新心情/情感使用。

import (
	"fmt"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/affection"
)

func rememberUserUtterance(content any) {
	text := strings.TrimSpace(messageText(PostMessage{Role: RoleUser, Content: content}))
	if text == "" {
		return
	}
	lastUserTextMu.Lock()
	lastUserText = text
	lastUserTextMu.Unlock()
}

// latestUserUtterance 优先取内存值；为空时从近期对话缓存倒序恢复。
func latestUserUtterance() string {
	lastUserTextMu.Lock()
	text := strings.TrimSpace(lastUserText)
	lastUserTextMu.Unlock()
	if text != "" {
		return text
	}
	cacheMu.Lock()
	records := append([]CachedMessage(nil), cachedMessages...)
	cacheMu.Unlock()
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Role != RoleUser {
			continue
		}
		content := strings.TrimSpace(records[i].Content)
		if content == "" {
			continue
		}
		if _, body := splitUserPrefix(content); strings.TrimSpace(body) != "" {
			return strings.TrimSpace(body)
		}
		return content
	}
	return ""
}

// latestUserName 最新用户发言的发送者名；优先内存值，为空时从对话缓存恢复。
func latestUserName() string {
	lastUserTextMu.Lock()
	text := lastUserText
	lastUserTextMu.Unlock()
	if sender, _ := splitUserPrefix(text); sender != "" {
		return sender
	}
	cacheMu.Lock()
	records := append([]CachedMessage(nil), cachedMessages...)
	cacheMu.Unlock()
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Role != RoleUser {
			continue
		}
		if sender := strings.TrimSpace(records[i].Sender); sender != "" {
			return sender
		}
		if sender, _ := splitUserPrefix(records[i].Content); sender != "" {
			return sender
		}
	}
	return ""
}

// latestAssistantUtterance 最近一条月华回应文本。
func latestAssistantUtterance() string {
	cacheMu.Lock()
	records := append([]CachedMessage(nil), cachedMessages...)
	cacheMu.Unlock()
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Role != RoleAssistant {
			continue
		}
		if text := strings.TrimSpace(records[i].Content); text != "" {
			return text
		}
	}
	return ""
}

// rememberEmotionMark 用户消息进入未读队列时记录已形成的情感标识；
// 该用户还没有形成情感认知时不记录，运行时消息也就不会出现标识。
func rememberEmotionMark(content any) {
	sender, _ := splitUserPrefix(messageText(PostMessage{Role: RoleUser, Content: content}))
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return
	}
	label, ok := affection.Label(sender)
	if !ok {
		return
	}
	unreadMu.Lock()
	unreadEmotionMarks[sender] = label
	unreadMu.Unlock()
}

// resetEmotionMarks 清理上一轮遗留的情感标识。
func resetEmotionMarks() {
	unreadMu.Lock()
	unreadEmotionMarks = map[string]string{}
	unreadMu.Unlock()
}

// emotionMarks 渲染本轮运行时消息要带的情感标识。
func emotionMarks() []string {
	unreadMu.Lock()
	names := make([]string, 0, len(unreadEmotionMarks))
	for name := range unreadEmotionMarks {
		names = append(names, name)
	}
	labels := make(map[string]string, len(unreadEmotionMarks))
	for name, label := range unreadEmotionMarks {
		labels[name] = label
	}
	unreadMu.Unlock()

	sort.Strings(names)
	marks := make([]string, 0, len(names))
	for _, name := range names {
		marks = append(marks, fmt.Sprintf("对「%s」的情感：%s", name, labels[name]))
	}
	return marks
}
