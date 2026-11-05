package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// maxGroupPoolSize 单个群聊缓存池的最大容量
const maxGroupPoolSize = 20

// maxForwardDepth 合并转发消息的最大展开层数（超过则折叠为占位符）
const maxForwardDepth = 3

// maxReplyQuoteLen 回复引用摘要的最大字符数
const maxReplyQuoteLen = 512

// HandleNapcatMessage 处理从 Napcat 接收到的消息（私聊 / 群聊）
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg napcat.NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		logger.SubError("LunarCore", "Napcat", "解析消息失败: %v", err)
		return
	}

	// 忽略自身发出的消息
	if napcatMsg.SelfID != 0 && napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	// 仅处理 message 事件，忽略 notice / request / meta_event / message_sent 等
	if napcatMsg.PostType != "" && napcatMsg.PostType != "message" {
		return
	}

	switch napcatMsg.MessageType {
	case "private":
		handlePrivateMessage(napcatMsg)
	case "group":
		handleGroupMessage(napcatMsg)
	}
}

// handleGroupMessage 处理群聊消息：内置缓存池与触发机制
func handleGroupMessage(msg napcat.NapcatMessage) {
	if !isAllowedTarget(msg.GroupID) {
		return
	}

	groupName := napcat.ResolveGroupName(msg.GroupID)
	memberName := resolveMemberName(msg.Sender)
	// 记录发言者名称，供入站 @ 渲染与出站 [对 xxx 说] 反向解析
	napcat.CacheMemberName(msg.GroupID, msg.UserID, memberName)
	napcat.WarmGroupMembers(msg.GroupID)

	content, hasImages, audioURLs := parseMessageSegments(msg.GroupID, msg.Message)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	redPacket := parseRawRedPacket(msg.Raw)
	if redPacket != nil {
		content = buildRedPacketText(redPacket)
		hasImages = false
		audioURLs = nil
	} else if strings.TrimSpace(contentToText(content)) == "" && !hasImages && len(audioURLs) == 0 {
		// 无可理解内容的空消息不入池不触发
		return
	}

	entry := GroupPoolEntry{Nickname: memberName, Content: content, HasImages: hasImages, AudioURLs: audioURLs, MessageID: msg.MessageID}

	selfID := msg.SelfID
	if selfID == 0 {
		selfID, _ = napcat.ResolveBotInfo()
	}
	selfStr := strconv.FormatInt(selfID, 10)
	// 引用月华的发言再说话，语意上仍是对月华讲话，与被直接 @ 等同
	atSelf := containsAtSelf(msg.Message, selfID) || replyQuotesSelf(msg.Message, selfID)
	mentioned := containsAnyKeyword(contentToText(content), bridgeConfig.BridgingGroupKeywords)
	triggerProbability := groupTriggerProbability()
	randomTrigger := triggerProbability > 0 && rand.Float64() < triggerProbability

	// 明确的 A@B：直接 @ 了群里其他具体成员而月华不在被 @ 之列，
	// 是成员两人之间的私聊，与月华无关——不入池也不触发，
	// 关键词与随机概率都不应把月华拉进一场与它无关的对话。
	// 合并转发与回复引用内部呈现的 @ 月华不受此判定影响：那是「转给月华看」的语义。
	if !atSelf && redPacket == nil && directAtOther(msg.Message, selfStr) {
		return
	}

	if atSelf || mentioned || redPacket != nil || randomTrigger {
		// 触发：将缓存池连同当前消息一并发送给月华（至多20条）
		entries := snapshotGroupPool(msg.GroupID)
		entries = append(entries, entry)
		if len(entries) > maxGroupPoolSize {
			entries = entries[len(entries)-maxGroupPoolSize:]
		}
		clearGroupPool(msg.GroupID)

		trigger := TriggerAt
		switch {
		case redPacket != nil:
			trigger = TriggerRedPacket
		case atSelf:
			trigger = TriggerAt
		case mentioned:
			trigger = TriggerKeyword
		default:
			trigger = TriggerRandom
		}

		req := BridgeRequest{
			Target:    BridgeTarget{ID: msg.GroupID, IsGroup: true, GroupName: groupName},
			Messages:  buildGroupMessages(groupName, entries),
			AudioURLs: collectGroupAudioURLs(entries),
			Trigger:   trigger,
		}
		// 被 @ 触发的回应，首次发言自动 @ 回发起者
		if atSelf {
			req.Target.ReplyToUserID = msg.UserID
		}
		// 随机触发：群里看不出月华在接谁的话，回应时自动引用相关性最高的一条消息
		if trigger == TriggerRandom {
			req.Target.QuoteCandidates = buildQuoteCandidates(entries)
		}

		if redPacket != nil && redPacket.IsPhrase {
			// 口令红包：等待随机 0.5~3 秒再推送，让月华复读口令领取红包
			go func() {
				time.Sleep(randomRedPacketDelay())
				enqueueRequest(req)
			}()
		} else {
			enqueueRequest(req)
		}
	} else {
		// 未触发：累积到缓存池
		addToGroupPool(msg.GroupID, groupName, entry)
	}
}

// isAllowedTarget 判断用户QQ号或群号是否在允许响应的白名单中
func isAllowedTarget(id int64) bool {
	for _, allowed := range bridgeConfig.BridgingUsers {
		if allowed == id {
			return true
		}
	}
	return false
}

// groupTriggerProbability 群聊随机应答概率：未配置时默认 0.3
func groupTriggerProbability() float64 {
	if bridgeConfig.BridgingGroupTriggerProbability != nil {
		return *bridgeConfig.BridgingGroupTriggerProbability
	}
	return 0.3
}

// containsAnyKeyword 判断文本是否包含关键词列表中的任意一个
func containsAnyKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		if kw != "" && strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// parseRawRedPacket 从 raw.elements 中识别红包（walletElement），非红包返回 nil
