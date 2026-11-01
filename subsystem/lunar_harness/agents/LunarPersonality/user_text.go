package LunarPersonality

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

// recentAffectionRounds 汇总最近若干轮「用户发言 + 月华回应」，按角色分组交给生活阶段逐人评估。
//
// 只收成对完整的轮次：没有回应的发言不构成情感交互；无发送者名的发言（如系统上下文）也跳过。
func recentAffectionRounds() []affection.Round {
	cacheMu.Lock()
	records := append([]CachedMessage(nil), cachedMessages...)
	cacheMu.Unlock()

	rounds := make([]affection.Round, 0, maxCacheMessages)
	perUser := make(map[string]int)
	reply := ""
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record.Role == RoleAssistant {
			if reply == "" {
				reply = strings.TrimSpace(record.Content)
			}
			continue
		}
		if reply == "" {
			continue
		}
		sender, body := splitUserPrefix(record.Content)
		if sender == "" {
			sender = strings.TrimSpace(record.Sender)
		}
		body = strings.TrimSpace(body)
		if sender == "" {
			// 工具产物等无法识别身份的用户侧内容，不打断配对
			continue
		}
		// 这段回应已被这条发言占用，无论是否计入窗口都要消费掉，避免配错到更早的发言
		roundReply := reply
		reply = ""
		if body == "" || perUser[sender] >= affectionRoundWindow {
			continue
		}
		perUser[sender]++
		rounds = append(rounds, affection.Round{
			Key:       record.ID,
			UserName:  sender,
			UserText:  body,
			ReplyText: roundReply,
		})
	}
	// 扫描自新到旧，翻转为时间顺序，使每个角色的窗口末尾是最新一轮。
	for left, right := 0, len(rounds)-1; left < right; left, right = left+1, right-1 {
		rounds[left], rounds[right] = rounds[right], rounds[left]
	}
	return rounds
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
