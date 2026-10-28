package lunar

// user_text.go 记录最新用户发言，供生活事件「更新心情」使用。

import "strings"

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
