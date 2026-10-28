package DialogueBridging

// private.go 私聊的「攒批 + 判定」机制
//
// 私聊过去是一发即回：用户先发一张图、紧跟着发一条文字要求，会被拆成两轮——
// 月华先对着图自己说一句，再被文字要求拉回来看一遍。群聊早就用缓存池 + 触发判定
// 解决了这个问题，私聊沿用同一套：消息先进池，命中关键词立即触发，否则按概率触发，
// 触发后还要等一个合并窗口（静默期）把紧随其后的消息一起带上再推送。
//
// 与群聊的差异只有两点：私聊是 1:1，不需要 @ 回执，也不需要按相关性自动引用。

import (
	"math/rand"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// privateTriggerProbability 私聊触发概率：未配置时默认 1（每条都答）
func privateTriggerProbability() float64 {
	if bridgeConfig.BridgingPrivateTriggerProbability != nil {
		value := *bridgeConfig.BridgingPrivateTriggerProbability
		if value < 0 {
			return 0
		}
		if value > 1 {
			return 1
		}
		return value
	}
	return 1
}

// privateBatchDelay 私聊合并窗口：未配置时用默认值
func privateBatchDelay() time.Duration {
	if bridgeConfig.BridgingPrivateBatchWindowMS != nil && *bridgeConfig.BridgingPrivateBatchWindowMS > 0 {
		return time.Duration(*bridgeConfig.BridgingPrivateBatchWindowMS) * time.Millisecond
	}
	return privateBatchWindow
}

// addToPrivatePool 把一条私聊消息加入缓存池（超限丢弃最老消息）
func addToPrivatePool(userID int64, entry GroupPoolEntry) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := append(privatePools[userID], entry)
	if len(pool) > privatePoolMaxSize {
		pool = pool[len(pool)-privatePoolMaxSize:]
	}
	privatePools[userID] = pool
}

// drainPrivatePool 取出并清空某用户的私聊缓存池
func drainPrivatePool(userID int64) []GroupPoolEntry {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := privatePools[userID]
	delete(privatePools, userID)
	if len(pool) == 0 {
		return nil
	}
	out := make([]GroupPoolEntry, len(pool))
	copy(out, pool)
	return out
}

// stopPrivateTimer 取消某用户尚未触发的合并窗口
func stopPrivateTimer(userID int64) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	if timer, ok := privateTimers[userID]; ok {
		timer.Stop()
		delete(privateTimers, userID)
	}
}

// schedulePrivateFlush 在合并窗口结束后把缓存池合并推送
//
// 每来一条新消息都重置定时器：用户连续发图 + 文字时，只有最后一条之后
// 静默满一个窗口才会真正推送，从而把这一串消息识别为同一次请求。
func schedulePrivateFlush(userID int64) {
	groupMutex.Lock()
	if timer, ok := privateTimers[userID]; ok {
		timer.Stop()
	}
	timer := time.AfterFunc(privateBatchDelay(), func() {
		groupMutex.Lock()
		delete(privateTimers, userID)
		groupMutex.Unlock()
		flushPrivatePool(userID, TriggerPrivate)
	})
	privateTimers[userID] = timer
	groupMutex.Unlock()
}

// flushPrivatePool 把缓存池合并成一个请求入队
func flushPrivatePool(userID int64, trigger TriggerKind) {
	stopPrivateTimer(userID)
	entries := drainPrivatePool(userID)
	if len(entries) == 0 {
		return
	}
	req := BridgeRequest{
		Target:    BridgeTarget{ID: userID, IsGroup: false},
		Messages:  buildPrivateMessages(entries),
		AudioURLs: collectGroupAudioURLs(entries),
		Trigger:   trigger,
	}
	logger.SubInfo("LunarCore", "Napcat", "私聊合并推送: 用户=%d, 条数=%d, 触发=%s", userID, len(entries), trigger)
	enqueueRequest(req)
}

// buildPrivateMessages 把私聊缓存池条目构建为 OpenAI 格式消息（带用户前缀）
func buildPrivateMessages(entries []GroupPoolEntry) []map[string]any {
	messages := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		prefix := ""
		if name := strings.TrimSpace(entry.Nickname); name != "" {
			prefix = "[用户: " + name + "]: "
		}
		messages = append(messages, buildUserMessage(prefix, entry.Content, entry.HasImages))
	}
	return messages
}

// handlePrivateMessage 处理私聊消息：入池后按「关键词立即触发 / 概率触发 + 合并窗口」推送
func handlePrivateMessage(msg NapcatMessage) {
	if !isAllowedTarget(msg.UserID) {
		logger.SubInfo("LunarCore", "Napcat", "忽略未授权用户 %d 的私聊消息", msg.UserID)
		return
	}

	nickname := resolveNickname(msg.UserID, msg.Sender)
	content, hasImages, audioURLs := parseMessageSegments(0, msg.Message)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	rp := parseRawRedPacket(msg.Raw)
	if rp != nil {
		content = buildRedPacketText(rp)
		hasImages = false
		audioURLs = nil
	} else if strings.TrimSpace(contentToText(content)) == "" && !hasImages && len(audioURLs) == 0 {
		// 无可理解内容的空消息（系统提示、空卡片等）直接忽略
		return
	}

	addToPrivatePool(msg.UserID, GroupPoolEntry{
		Nickname:  nickname,
		Content:   content,
		HasImages: hasImages,
		AudioURLs: audioURLs,
		MessageID: msg.MessageID,
	})

	if rp != nil {
		stopPrivateTimer(msg.UserID)
		entries := drainPrivatePool(msg.UserID)
		req := BridgeRequest{
			Target:    BridgeTarget{ID: msg.UserID, IsGroup: false},
			Messages:  buildPrivateMessages(entries),
			AudioURLs: collectGroupAudioURLs(entries),
			Trigger:   TriggerRedPacket,
		}
		if rp.IsPhrase {
			// 口令红包：等待随机 0.5~3 秒再推送，让月华复读口令领取红包
			go func() {
				time.Sleep(randomRedPacketDelay())
				enqueueRequest(req)
			}()
			return
		}
		enqueueRequest(req)
		return
	}

	// 关键词命中：不再等待窗口，连同池中已有消息立即推送
	if containsAnyKeyword(contentToText(content), bridgeConfig.BridgingGroupKeywords) {
		flushPrivatePool(msg.UserID, TriggerKeyword)
		return
	}
	if rand.Float64() < privateTriggerProbability() {
		schedulePrivateFlush(msg.UserID)
	}
}
