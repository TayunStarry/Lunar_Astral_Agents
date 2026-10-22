package napcat

// 消息解析与单线程收发流程

import (
	file "CorePersonality/engine/persistence"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// maxGroupPoolSize 单个群聊缓存池的最大容量
const maxGroupPoolSize = 20

// maxForwardDepth 合并转发消息的最大展开层数（超过则折叠为占位符）
const maxForwardDepth = 3

// maxReplyQuoteLen 回复引用摘要的最大字符数
const maxReplyQuoteLen = 120

// HandleNapcatMessage 处理从 Napcat 接收到的消息（私聊 / 群聊）
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "解析消息失败: %v", err)
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

// handlePrivateMessage 处理私聊消息：即时构建请求并入队
func handlePrivateMessage(msg NapcatMessage) {
	if !isAllowedTarget(msg.UserID) {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "忽略未授权用户 %d 的私聊消息", msg.UserID)
		return
	}

	nickname := resolveNickname(msg.UserID, msg.Sender)
	content, hasImages, videoURLs, audioURLs := parseMessageSegments(0, msg.Message)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	rp := parseRawRedPacket(msg.Raw)
	if rp != nil {
		content = buildRedPacketText(rp)
		hasImages = false
		videoURLs = nil
		audioURLs = nil
	} else if strings.TrimSpace(contentToText(content)) == "" && !hasImages && len(videoURLs) == 0 && len(audioURLs) == 0 {
		// 无可理解内容的空消息（系统提示、空卡片等）直接忽略
		return
	}

	req := BridgeRequest{
		Target:    BridgeTarget{ID: msg.UserID, IsGroup: false},
		Messages:  []map[string]any{buildUserMessage("[用户: "+nickname+"]: ", content, hasImages)},
		VideoURLs: videoURLs,
		AudioURLs: audioURLs,
	}

	if rp != nil && rp.IsPhrase {
		// 口令红包：等待随机 0.5~3 秒再推送，让月华复读口令领取红包
		go func() {
			time.Sleep(randomRedPacketDelay())
			enqueueRequest(req)
		}()
		return
	}
	enqueueRequest(req)
}

// handleGroupMessage 处理群聊消息：内置缓存池与触发机制
func handleGroupMessage(msg NapcatMessage) {
	if !isAllowedTarget(msg.GroupID) {
		return
	}

	groupName := resolveGroupName(msg.GroupID)
	memberName := resolveMemberName(msg.Sender)
	// 记录发言者名称，供入站 @ 渲染与出站 [对 xxx 说] 反向解析
	cacheMemberName(msg.GroupID, msg.UserID, memberName)
	warmGroupMembers(msg.GroupID)

	content, hasImages, videoURLs, audioURLs := parseMessageSegments(msg.GroupID, msg.Message)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	redPacket := parseRawRedPacket(msg.Raw)
	if redPacket != nil {
		content = buildRedPacketText(redPacket)
		hasImages = false
		videoURLs = nil
		audioURLs = nil
	} else if strings.TrimSpace(contentToText(content)) == "" && !hasImages && len(videoURLs) == 0 && len(audioURLs) == 0 {
		// 无可理解内容的空消息不入池不触发
		return
	}

	entry := GroupPoolEntry{Nickname: memberName, Content: content, HasImages: hasImages, VideoURLs: videoURLs, AudioURLs: audioURLs}

	selfID := msg.SelfID
	if selfID == 0 {
		selfID, _ = resolveBotInfo()
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

		req := BridgeRequest{
			Target:    BridgeTarget{ID: msg.GroupID, IsGroup: true, GroupName: groupName},
			Messages:  buildGroupMessages(groupName, entries),
			VideoURLs: collectGroupVideoURLs(entries),
			AudioURLs: collectGroupAudioURLs(entries),
		}
		// 被 @ 触发的回应，首次发言自动 @ 回发起者
		if atSelf {
			req.Target.ReplyToUserID = msg.UserID
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
func parseRawRedPacket(raw json.RawMessage) *RedPacketInfo {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Elements []struct {
			Wallet *WalletElement `json:"walletElement"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	for _, e := range payload.Elements {
		if e.Wallet == nil {
			continue
		}
		return &RedPacketInfo{
			IsRedPacket: true,
			IsPhrase:    e.Wallet.RedChannel == 32 || e.Wallet.MsgType == 6,
			Blessing:    walletBlessing(e.Wallet),
			BillNo:      e.Wallet.BillNo,
		}
	}
	return nil
}

// walletBlessing 提取红包祝福语：receiver.title 优先，notice 去掉 "[QQ红包]" 前缀兜底
func walletBlessing(w *WalletElement) string {
	if w.Receiver.Title != "" {
		return w.Receiver.Title
	}
	if notice := strings.TrimPrefix(w.Receiver.Notice, "[QQ红包]"); notice != "" {
		return notice
	}
	return w.Receiver.Content
}

// buildRedPacketText 将红包信息渲染为文本（口令红包附带复读领取指令）
func buildRedPacketText(rp *RedPacketInfo) string {
	if rp.IsPhrase {
		return fmt.Sprintf("[口令红包] 口令：『%s』，请直接复读口令领取红包 ", rp.Blessing)
	}
	return fmt.Sprintf("[红包] 祝福语: %s ", rp.Blessing)
}

// randomRedPacketDelay 领取口令红包前的随机等待时间（0.5~3 秒）
func randomRedPacketDelay() time.Duration {
	return time.Duration(500+rand.Intn(2500)) * time.Millisecond
}

// resolveNickname 解析私聊用户昵称：优先 sender.nickname，其次 card，最后通过接口查询
func resolveNickname(userID int64, sender Sender) string {
	if sender.Nickname != "" {
		return sender.Nickname
	}
	if sender.Card != "" {
		return sender.Card
	}
	if name, err := getStrangerNickname(userID); err == nil && name != "" {
		return name
	}
	return strconv.FormatInt(userID, 10)
}

// resolveMemberName 解析群聊发言用户昵称：群名片优先，昵称兜底
func resolveMemberName(sender Sender) string {
	if sender.Card != "" {
		return sender.Card
	}
	return sender.Nickname
}

// containsAtSelf 判断消息段中是否 @了当前账号
func containsAtSelf(segments []MessageSegment, selfID int64) bool {
	selfStr := strconv.FormatInt(selfID, 10)
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var atData AtData
		if json.Unmarshal(segment.Data, &atData) == nil && atData.QQ == selfStr {
			return true
		}
	}
	return false
}

// directAtOther 判断消息是否直接 @ 了群里除月华之外的其他具体成员（A@B 私聊语义）。
// @全体成员（qq=all / 0）面向全群广播，不算两人私聊；仅检查顶层消息段，
// 合并转发与回复引用内部的 @ 由渲染层呈现，不参与该判定。
func directAtOther(segments []MessageSegment, selfStr string) bool {
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var atData AtData
		if json.Unmarshal(segment.Data, &atData) != nil {
			continue
		}
		if atData.QQ == selfStr || atData.QQ == "" || atData.QQ == "all" || atData.QQ == "0" {
			continue
		}
		return true
	}
	return false
}

// replyQuotesSelf 判断回复引用的原始消息是否为月华自己所发：
// 「引用月华的话再发言」在语意上仍是对月华说话，应当转给月华。
func replyQuotesSelf(segments []MessageSegment, selfID int64) bool {
	for _, segment := range segments {
		if segment.Type != "reply" {
			continue
		}
		var replyData ReplyData
		if json.Unmarshal(segment.Data, &replyData) != nil {
			continue
		}
		id := rawIDString(replyData.ID)
		if id == "" {
			continue
		}
		detail, err := getMessageDetail(id)
		if err != nil || detail == nil {
			continue
		}
		if detail.Sender.UserID == selfID {
			return true
		}
	}
	return false
}

// resolveGroupName 解析并缓存群名称
func resolveGroupName(groupID int64) string {
	groupMutex.Lock()
	if name, ok := groupNameCache[groupID]; ok && name != "" {
		groupMutex.Unlock()
		return name
	}
	groupMutex.Unlock()

	name, err := getGroupName(groupID)
	if err != nil || name == "" {
		name = strconv.FormatInt(groupID, 10)
	}

	groupMutex.Lock()
	groupNameCache[groupID] = name
	groupMutex.Unlock()
	return name
}

// snapshotGroupPool 获取群聊缓存池的快照
func snapshotGroupPool(groupID int64) []GroupPoolEntry {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		return nil
	}
	out := make([]GroupPoolEntry, len(pool.Entries))
	copy(out, pool.Entries)
	return out
}

// addToGroupPool 添加消息到群聊缓存池，超出容量时抛弃最老消息
func addToGroupPool(groupID int64, groupName string, entry GroupPoolEntry) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		pool = &GroupPool{GroupID: groupID, GroupName: groupName}
		groupPools[groupID] = pool
	}
	pool.Entries = append(pool.Entries, entry)
	if len(pool.Entries) > maxGroupPoolSize {
		pool.Entries = pool.Entries[len(pool.Entries)-maxGroupPoolSize:]
	}
}

// clearGroupPool 清空指定群聊的缓存池
func clearGroupPool(groupID int64) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	delete(groupPools, groupID)
}

// buildGroupMessages 将群聊缓存池条目构建为 OpenAI 格式消息（带群聊前缀）
func buildGroupMessages(groupName string, entries []GroupPoolEntry) []map[string]any {
	messages := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		prefix := fmt.Sprintf("[群聊: %s][用户: %s]: ", groupName, e.Nickname)
		messages = append(messages, buildUserMessage(prefix, e.Content, e.HasImages))
	}
	return messages
}

// collectGroupVideoURLs 汇总群聊缓存池条目中的视频地址
func collectGroupVideoURLs(entries []GroupPoolEntry) []string {
	var urls []string
	for _, e := range entries {
		urls = append(urls, e.VideoURLs...)
	}
	return urls
}

// collectGroupAudioURLs 汇总群聊缓存池条目中的语音/音频地址
func collectGroupAudioURLs(entries []GroupPoolEntry) []string {
	var urls []string
	for _, e := range entries {
		urls = append(urls, e.AudioURLs...)
	}
	return urls
}

// contentToText 从解析结果（string 或 多模态数组）中提取纯文本
func contentToText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]map[string]any); ok {
		var sb strings.Builder
		for _, item := range arr {
			if item["type"] == "text" {
				if t, ok := item["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// buildUserMessage 将内容与发送者前缀组装为单条 OpenAI 消息
func buildUserMessage(prefix string, content any, hasImages bool) map[string]any {
	if hasImages {
		arr, _ := content.([]map[string]any)
		withPrefix := append([]map[string]any{{"type": "text", "text": prefix}}, arr...)
		return map[string]any{"role": "user", "content": withPrefix}
	}
	text, _ := content.(string)
	return map[string]any{"role": "user", "content": prefix + text}
}

// enqueueRequest 将请求加入队列并尝试推进
func enqueueRequest(req BridgeRequest) {
	flowMutex.Lock()
	requestQueue = append(requestQueue, req)
	flowMutex.Unlock()
	pumpNext()
}

// pumpNext 在空闲时（无待回应请求）推送队列头的请求给月华
func pumpNext() {
	flowMutex.Lock()
	defer flowMutex.Unlock()

	if awaitingResponse || len(requestQueue) == 0 {
		return
	}

	req := requestQueue[0]
	requestQueue = requestQueue[1:]
	currentTarget = req.Target
	awaitingResponse = true

	if SendMessageToAgent != nil {
		SendMessageToAgent(req.Messages)
	} else {
		awaitingResponse = false
		LoggerGeneral.SubError("LunarCore", "Napcat", "SendMessageToAgent 回调未注册，无法推送消息")
	}
	if SendVideoToAgent != nil && len(req.VideoURLs) > 0 {
		SendVideoToAgent(req.VideoURLs)
	}
	if SendAudioToAgent != nil && len(req.AudioURLs) > 0 {
		SendAudioToAgent(req.AudioURLs)
	}
}

// parseMessageSegments 解析消息段列表，返回 (内容, 是否含图片, 视频地址列表, 语音/音频地址列表)
// 纯文本返回 string，包含图片返回 []map[string]any；视频与语音地址分别写入后两个返回值
// groupID 用于 @ 目标与回复引用的成员名称解析（私聊传 0）
func parseMessageSegments(groupID int64, segments []MessageSegment) (any, bool, []string, []string) {
	var contentArray []map[string]any
	var contentStr string
	var hasImages bool
	var videoURLs []string
	var audioURLs []string

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				appendContent(&contentArray, &contentStr, textData.Text)
			}
		case "at":
			var atData AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				appendContent(&contentArray, &contentStr, "[对 "+resolveAtDisplay(groupID, atData.QQ)+" 说] ")
			}
		case "reply":
			var replyData ReplyData
			if json.Unmarshal(segment.Data, &replyData) == nil {
				appendContent(&contentArray, &contentStr, renderReplyQuote(groupID, rawIDString(replyData.ID)))
			}
		case "image":
			var imageData ImageData
			if json.Unmarshal(segment.Data, &imageData) == nil {
				imgURL := resolveImageURL(imageData)
				if imgURL != "" {
					hasImages = true
					markMultimedia(&contentArray, &contentStr)
					contentArray = append(contentArray, map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": imgURL},
					})
				}
			}
		case "video":
			var videoData VideoData
			if json.Unmarshal(segment.Data, &videoData) == nil {
				if source := resolveVideoSource(videoData); source != "" {
					videoURLs = append(videoURLs, source)
				}
				appendContent(&contentArray, &contentStr, "[视频] ")
			}
		case "record":
			var recordData RecordData
			if json.Unmarshal(segment.Data, &recordData) == nil {
				if source := resolveAudioSource(recordData); source != "" {
					audioURLs = append(audioURLs, source)
				}
			}
			appendContent(&contentArray, &contentStr, "[语音] ")
		case "face":
			if text := faceDisplayText(segment.Data); text != "" {
				appendContent(&contentArray, &contentStr, "[表情: "+text+"] ")
			} else {
				appendContent(&contentArray, &contentStr, "[表情] ")
			}
		case "json":
			var jsonData JsonData
			if json.Unmarshal(segment.Data, &jsonData) == nil {
				if title := extractJsonCardTitle(jsonData.Data); title != "" {
					appendContent(&contentArray, &contentStr, "[卡片: "+title+"] ")
				} else {
					appendContent(&contentArray, &contentStr, "[卡片] ")
				}
			}
		case "share":
			var shareData ShareData
			if json.Unmarshal(segment.Data, &shareData) == nil {
				if shareData.Title != "" {
					appendContent(&contentArray, &contentStr, "[链接: "+shareData.Title+"] ")
				} else {
					appendContent(&contentArray, &contentStr, "[链接] ")
				}
			}
		case "file":
			var fileData FileData
			if json.Unmarshal(segment.Data, &fileData) == nil {
				fileText, isImage, dataURI := processFileSegment(fileData)
				if isImage {
					hasImages = true
					markMultimedia(&contentArray, &contentStr)
					contentArray = append(contentArray, map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": dataURI},
					})
				} else {
					appendContent(&contentArray, &contentStr, fileText)
				}
			}
		case "forward":
			var forwardData ForwardData
			if json.Unmarshal(segment.Data, &forwardData) == nil {
				// 转发内图片已注册进共享图片缓存，占位标签内嵌在文本中（不注入多模态项，避免大转发包撑爆上下文）
				forwardText := processForwardSegment(groupID, rawIDString(forwardData.ID), 0)
				appendContent(&contentArray, &contentStr, forwardText)
			}
		default:
			// 忽略其余消息段类型
		}
	}

	if hasImages {
		return contentArray, true, videoURLs, audioURLs
	}
	return contentStr, false, videoURLs, audioURLs
}

// renderReplyQuote 通过 get_msg 还原被回复消息，渲染为 [回复 <发送者>: <内容摘要>]
func renderReplyQuote(groupID int64, messageID string) string {
	if messageID == "" {
		return "[回复] "
	}
	detail, err := getMessageDetail(messageID)
	if err != nil || detail == nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "获取回复引用消息失败: %v", err)
		return "[回复] "
	}

	sender := detail.Sender.Card
	if sender == "" {
		sender = detail.Sender.Nickname
	}
	if sender == "" {
		sender = strconv.FormatInt(detail.Sender.UserID, 10)
	}
	segments := detail.Message
	if len(segments) == 0 {
		segments = detail.Content
	}
	summary := extractSegmentText(groupID, 0, segments, false)
	if summary == "" {
		return "[回复] "
	}
	if utf8.RuneCountInString(summary) > maxReplyQuoteLen {
		summary = string([]rune(summary)[:maxReplyQuoteLen]) + "…"
	}
	return "[回复 " + sender + ": " + summary + "] "
}

// resolveImageURL 获取图片的可访问地址：优先使用 url，其次直接使用 http 引用，最后通过接口下载为 base64 data URI
func resolveImageURL(imageData ImageData) string {
	if imageData.URL != "" {
		return imageData.URL
	}
	if imageData.File != "" {
		if strings.HasPrefix(imageData.File, "http://") || strings.HasPrefix(imageData.File, "https://") {
			return imageData.File
		}
		if bytes, err := getImageContent(imageData.File); err == nil && len(bytes) > 0 {
			// MIME 按文件头嗅探：QQ 常把 GIF 动图当作 png/jpg 下发，
			// 硬编码 image/png 会让后续入库把动图存成 .png，按扩展名判定时退化为静态图。
			mimeType := sniffImageMIME(bytes)
			if mimeType == "" {
				mimeType = "image/png"
			}
			return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(bytes)
		}
	}
	return ""
}

// admitForwardImage 把合并转发内的图片收进统一资源归档层并返回持久句柄。
//
// 合并转发与普通消息不同：普通图片会作为多模态内容项直接进入思考链，而转发内的图片
// 只需要“可回引”的句柄。此处必须同时兼容两种来源形态：
//   - 远程直链（image.url）：按 url 来源下载；
//   - 仅本地文件引用（image.file）：get_image 下载后得到 data URI，必须按 data 来源解码，
//     不能把 data: 当成 http URL 去请求——那会直接失败并退化成 [图片]。
//
// 直链下载失败时再尝试用 file 引用兜底，避免 QQ CDN 地址过期导致整条转发图全部丢失。
func admitForwardImage(imageData ImageData) (string, bool) {
	candidates := make([]string, 0, 2)
	if imgURL := resolveImageURL(imageData); imgURL != "" {
		candidates = append(candidates, imgURL)
	}
	// 直链存在时 resolveImageURL 不会读取 file；补一份本地下载结果作为兜底
	if imageData.URL != "" && imageData.File != "" && !strings.HasPrefix(imageData.File, "http://") && !strings.HasPrefix(imageData.File, "https://") {
		if raw, err := getImageContent(imageData.File); err == nil && len(raw) > 0 {
			mimeType := sniffImageMIME(raw)
			if mimeType == "" {
				mimeType = "image/png"
			}
			candidates = append(candidates, "data:"+mimeType+";base64,"+base64.StdEncoding.EncodeToString(raw))
		}
	}

	for _, source := range candidates {
		sourceKind := file.SourceKindURL
		if strings.HasPrefix(source, "data:") {
			sourceKind = file.SourceKindData
		}
		// 归档层按「建议文件名」的扩展名推断类型：data URI 与无扩展名的 QQ CDN
		// 直链都必须补一个图片扩展名，否则会被当成文档（或直接失败退化成 [图片]）。
		name := forwardImageName(source)
		// 转发图只为还原对话记录服务：ephemeral 保留等级，配额吃紧时优先回收本体，
		// 句柄降级为描述后依然可引用；同图重复转发按内容去重，不重复占句柄。
		res, err := file.ResourceAdmit(file.AdmitInput{
			SourceKind: sourceKind,
			Source:     source,
			Name:       name,
			Origin:     "channel",
			Retain:     file.RetainEphemeral,
			Dedup:      true,
		})
		if err == nil && res.Handle != "" {
			return res.Handle, true
		}
		if err != nil {
			LoggerGeneral.SubWarn("LunarCore", "Napcat", "合并转发图片入库失败（%s）: %v", sourceKind, err)
		}
	}
	return "", false
}

// resolveVideoSource 获取视频的可访问地址（优先 url，其次通过 get_file 下载）
func resolveVideoSource(videoData VideoData) string {
	if videoData.URL != "" {
		return videoData.URL
	}
	if videoData.File != "" {
		if strings.HasPrefix(videoData.File, "http://") || strings.HasPrefix(videoData.File, "https://") {
			return videoData.File
		}
		if source, err := getVideoSource(videoData.File); err == nil && source != "" {
			return source
		}
	}
	return ""
}

// resolveAudioSource 获取语音的可访问地址
// 优先 url 直链；其次通过 get_record 转码（QQ 语音为 silk/amr 格式，由 NapCat 转为通用音频）；
// 最后退化 get_file 获取原始文件（url / 本地路径 / base64 data URI）
func resolveAudioSource(recordData RecordData) string {
	if recordData.URL != "" {
		return recordData.URL
	}
	if recordData.File == "" {
		return ""
	}
	if strings.HasPrefix(recordData.File, "http://") || strings.HasPrefix(recordData.File, "https://") {
		return recordData.File
	}
	// get_record 转码：mp3 通用格式，语音理解管线可识别
	if resp, err := callNapcatAPI("/get_record", map[string]any{"file": recordData.File, "out_format": "mp3"}); err == nil {
		var data GetFileResponse
		if json.Unmarshal(resp.Data, &data) == nil {
			if data.Base64 != "" {
				return "data:audio/mp3;base64," + data.Base64
			}
			if data.URL != "" {
				return data.URL
			}
			if data.File != "" {
				return data.File
			}
		}
	} else {
		LoggerGeneral.SubError("LunarCore", "Napcat", "get_record 转码失败: %v", err)
	}
	// 退化 get_file 获取原始语音文件
	if source, err := getVideoSource(recordData.File); err == nil && source != "" {
		return source
	}
	return ""
}

// processFileSegment 处理文件消息段，返回 (文本描述, 是否图片, 图片dataURI)
func processFileSegment(fileData FileData) (string, bool, string) {
	fileName := fileData.FileName
	if fileName == "" {
		fileName = fileData.Name
	}
	if fileName == "" {
		fileName = fileData.File
	}

	// 未提供文件标识时，仅回传名称与大小信息
	if fileData.FileID == "" && fileData.File == "" && fileData.URL == "" {
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	var data []byte
	var err error
	if fileData.FileID != "" || fileData.File != "" {
		data, err = getFileContent(fileData.FileID, fileData.File)
	} else {
		data, err = fetchFileBytes(fileData.URL)
	}
	if err != nil || len(data) == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "下载文件失败: %v", err)
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	// 图片文件 → 作为多媒体内容处理（扩展名缺失时按文件头嗅探）
	if mimeType := imageMIME(fileName); mimeType != "" {
		return "", true, "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	if mimeType := sniffImageMIME(data); mimeType != "" {
		return "", true, "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}

	// 其余可归档文件 → 在 Go 侧直接入库，上下文只传递规范句柄
	// （文本/表格可在思考链中按句柄读取；二进制与未知类型仍回退为名称与大小描述）
	if file.KindOfExt(strings.ToLower(filepath.Ext(fileName))) == "" {
		return describeFile(fileName, fileData.FileSize), false, ""
	}
	res, err := file.ResourceAdmit(file.AdmitInput{
		SourceKind: file.SourceKindBytes,
		Data:       data,
		Name:       fileName,
		Origin:     file.OriginChannel,
		Retain:     file.RetainNormal,
	})
	if err != nil || res.Handle == "" {
		LoggerGeneral.SubError("LunarCore", "Napcat", "文件入库失败: %v", err)
		return describeFile(fileName, fileData.FileSize), false, ""
	}
	name := res.Name
	if name == "" {
		name = fileName
	}
	return "[已收录] " + res.Handle + " " + name + " ", false, ""
}

// describeFile 构建文件的名称与大小信息描述
func describeFile(fileName string, sizeRaw json.RawMessage) string {
	var sb strings.Builder
	sb.WriteString("[文件]")
	if fileName != "" {
		sb.WriteString(" 名称: ")
		sb.WriteString(fileName)
	}
	if size := fileSizeText(sizeRaw); size != "" {
		sb.WriteString(" 大小: ")
		sb.WriteString(size)
	}
	sb.WriteString(" ")
	return sb.String()
}

// fileSizeText 将 file_size 字段（可能为数字或字符串）解析为字符串
func fileSizeText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return ""
}

// imageMIME 通过扩展名判断并返回图片 MIME 类型，非图片返回空字符串
func imageMIME(fileName string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// sniffImageMIME 通过文件头识别常见图片格式，非图片返回空字符串
func sniffImageMIME(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	case len(data) >= 3 && bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(data) >= 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))):
		return "image/gif"
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case len(data) >= 2 && bytes.HasPrefix(data, []byte("BM")):
		return "image/bmp"
	default:
		return ""
	}
}

// dataURIExt 从 data:image/... URI 的 MIME 推断扩展名（未知时回退 .png）
func dataURIExt(dataURI string) string {
	meta := dataURI
	if end := strings.IndexAny(meta, ";,"); end >= 0 {
		meta = meta[:end]
	}
	switch strings.ToLower(strings.TrimPrefix(meta, "data:")) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	default:
		return ".png"
	}
}

// forwardImageName 为转发图片生成带图片扩展名的建议文件名。
//
// QQ CDN 直链经常没有扩展名；如果没有扩展名，归档层会按文档处理。
// 这里未知格式一律先用 .jpg，归档层随后会按文件头嗅探把 GIF/WebP 等纠正过来。
func forwardImageName(source string) string {
	if strings.HasPrefix(source, "data:") {
		return "forward" + dataURIExt(source)
	}
	pathPart := source
	if cut := strings.IndexAny(pathPart, "?#"); cut >= 0 {
		pathPart = pathPart[:cut]
	}
	switch ext := strings.ToLower(filepath.Ext(pathPart)); ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return "forward" + ext
	default:
		return "forward.jpg"
	}
}

// extractJsonCardTitle 从 JSON 卡片数据中提取标题（小程序 / 分享卡片 / 邀请等）
func extractJsonCardTitle(data string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(data), &payload) != nil {
		return ""
	}
	if title, ok := payload["prompt"].(string); ok && title != "" {
		return title
	}
	if meta, ok := payload["meta"].(map[string]any); ok {
		for _, v := range meta {
			if m, ok := v.(map[string]any); ok {
				if title, ok := m["title"].(string); ok && title != "" {
					return title
				}
			}
		}
	}
	if title, ok := payload["title"].(string); ok && title != "" {
		return title
	}
	return ""
}

// processForwardSegment 展开合并转发消息（含嵌套转发递归），拼接为文本描述
// 子消息中的图片注册进共享图片缓存，正文内嵌 [图片.00x] 占位标签（按标签取回，不再注入多模态项）
func processForwardSegment(groupID int64, id string, depth int) string {
	if id == "" {
		return "[转发消息] "
	}
	if depth >= maxForwardDepth {
		return "[嵌套转发消息（层级过深，省略）] "
	}
	messages, err := getForwardMessageContent(id)
	if err != nil || len(messages) == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "获取合并转发消息失败: %v", err)
		return "[转发消息] "
	}

	indent := strings.Repeat("    ", depth)
	var sb strings.Builder
	if depth == 0 {
		sb.WriteString("[转发消息]\n")
	}
	for _, msg := range messages {
		sender := msg.Sender.Card
		if sender == "" {
			sender = msg.Sender.Nickname
		}
		if sender == "" {
			sender = strconv.FormatInt(msg.Sender.UserID, 10)
		}
		segments := msg.Message
		if len(segments) == 0 {
			segments = msg.Content
		}
		// 纯转发节点：递归展开为嵌套块
		if nestedID := findNestedForward(segments); nestedID != "" {
			sb.WriteString(indent)
			sb.WriteString(sender)
			sb.WriteString(" 转发了以下内容：\n")
			sb.WriteString(processForwardSegment(groupID, nestedID, depth+1))
			continue
		}
		sb.WriteString(indent)
		sb.WriteString(sender)
		sb.WriteString(": ")
		sb.WriteString(extractSegmentText(groupID, depth, segments, true))
		sb.WriteString("\n")
	}
	return sb.String()
}

// findNestedForward 若消息段为单个合并转发节点，返回其 ID，否则返回空
func findNestedForward(segments []MessageSegment) string {
	if len(segments) != 1 || segments[0].Type != "forward" {
		return ""
	}
	var forwardData ForwardData
	if json.Unmarshal(segments[0].Data, &forwardData) == nil {
		return rawIDString(forwardData.ID)
	}
	return ""
}

// rawIDString 提取 JSON id 字段的字符串值（兼容 "123" 与 123 两种形态）
func rawIDString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatInt(n, 10)
	}
	return ""
}

// extractSegmentText 从消息段列表中提取文本摘要（回复引用与合并转发子消息共用）
// resolveImages 为 true 时（合并转发展开）图片注册进共享图片缓存并内嵌 [图片.00x] 占位标签；
// 为 false 时（回复引用摘要）图片仅显示为 [图片] 字样
func extractSegmentText(groupID int64, depth int, segments []MessageSegment, resolveImages bool) string {
	var sb strings.Builder
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				sb.WriteString(textData.Text)
			}
		case "at":
			var atData AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				sb.WriteString("[对 ")
				sb.WriteString(resolveAtDisplay(groupID, atData.QQ))
				sb.WriteString(" 说] ")
			}
		case "image":
			registered := false
			var imageData ImageData
			if resolveImages && json.Unmarshal(segment.Data, &imageData) == nil {
				if handle, ok := admitForwardImage(imageData); ok {
					sb.WriteString("[")
					sb.WriteString(handle)
					sb.WriteString("]")
					registered = true
				}
			}
			if !registered {
				sb.WriteString("[图片]")
			}
		case "video":
			sb.WriteString("[视频]")
		case "record":
			sb.WriteString("[语音]")
		case "file":
			var fileData FileData
			if json.Unmarshal(segment.Data, &fileData) == nil {
				name := fileData.FileName
				if name == "" {
					name = fileData.Name
				}
				if name == "" {
					name = fileData.File
				}
				if name != "" {
					sb.WriteString("[文件: ")
					sb.WriteString(name)
					sb.WriteString("]")
				} else {
					sb.WriteString("[文件]")
				}
			} else {
				sb.WriteString("[文件]")
			}
		case "face":
			if text := faceDisplayText(segment.Data); text != "" {
				sb.WriteString("[表情: ")
				sb.WriteString(text)
				sb.WriteString("]")
			} else {
				sb.WriteString("[表情]")
			}
		case "forward":
			var forwardData ForwardData
			if json.Unmarshal(segment.Data, &forwardData) == nil {
				sb.WriteString(processForwardSegment(groupID, rawIDString(forwardData.ID), depth+1))
			}
		case "json":
			sb.WriteString("[卡片]")
		case "share":
			sb.WriteString("[链接]")
		}
	}
	return sb.String()
}

// appendContent 根据当前内容格式追加文本
// 如果已经是数组格式（有图片），追加为 text 类型元素；否则追加到纯字符串
func appendContent(contentArray *[]map[string]any, contentStr *string, text string) {
	if len(*contentArray) > 0 {
		*contentArray = append(*contentArray, map[string]any{
			"type": "text",
			"text": text,
		})
	} else {
		*contentStr += text
	}
}

// markMultimedia 首次遇到多媒体时将已累积的纯文本迁移到数组格式
func markMultimedia(contentArray *[]map[string]any, contentStr *string) {
	if len(*contentArray) == 0 && *contentStr != "" {
		*contentArray = append(*contentArray, map[string]any{
			"type": "text",
			"text": *contentStr,
		})
		*contentStr = ""
	}
}
