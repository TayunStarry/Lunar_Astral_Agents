package thinking

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// writeMessage 写入消息（文本或多模态内容）
//
// source 为宿主侧来源标记：愿望到期触发的上下文只进入模型上下文，
// 不写入前端可见的对话缓存，避免「系统内部提醒」以用户气泡出现在聊天界面。
func writeMessage(role, source string, content any) {
	normalized := normalizeIncomingContent(content)
	message := PostMessage{Role: role, Content: normalized}
	if role == RoleUser && source != wishSource {
		appendUserMessage(message.Content)
		rememberUserUtterance(message.Content)
		rememberEmotionMark(message.Content)
	}
	unreadMu.Lock()
	unreadContext = append(unreadContext, message)
	unreadMu.Unlock()

	for _, item := range asItems(message.Content) {
		switch item.Type {
		case ContentText:
			logger.Info(lunarCoreModule, "收到文本: %s", item.Text)
		case ContentImageURL:
			if item.ImageURL != nil {
				logger.Info(lunarCoreModule, "收到图片: %s", textutil.TruncateRunes(item.ImageURL.URL, 512))
			}
		case ContentImageStatic:
			if item.ImageStatic != nil {
				logger.Info(lunarCoreModule, "收到静态图信封: %s", textutil.TruncateRunes(item.ImageStatic.Name, 128))
			}
		case ContentImageAnimated:
			if item.ImageAnimated != nil {
				logger.Info(lunarCoreModule, "收到动图信封: %s", textutil.TruncateRunes(item.ImageAnimated.Name, 128))
			}
		case ContentVideo:
			if item.Video != nil {
				logger.Info(lunarCoreModule, "收到视频信封: %s", textutil.TruncateRunes(item.Video.Name, 128))
			}
		case ContentAudio:
			if item.Audio != nil {
				logger.Info(lunarCoreModule, "收到音频信封: %s", textutil.TruncateRunes(item.Audio.Name, 128))
			}
		case ContentFile:
			if item.File != nil {
				logger.Info(lunarCoreModule, "收到文件信封: %s", textutil.TruncateRunes(item.File.Name, 128))
			}
		case ContentWebpage:
			if item.Webpage != nil {
				logger.Info(lunarCoreModule, "收到网页信封: %s", textutil.TruncateRunes(item.Webpage.URL, 512))
			}
		}
	}
}

// normalizeIncomingContent 把宿主下发的消息内容统一归一为「字符串」或「[]ContentItem」
//
// 各入口原始类型并不一致：QQ 桥接是 []map[string]any，HTTP 入口是 []any，
// 内部构造是 []ContentItem。这里统一成内容项数组，后续的图片赋签、媒体入库、
// 窗口降级才有共同的操作对象。
func normalizeIncomingContent(content any) any {
	if content == nil {
		return ""
	}
	if text, ok := content.(string); ok {
		return text
	}
	if object, ok := content.(map[string]any); ok {
		if kind, _ := object["type"].(string); kind == ContentText {
			if text, ok := object["text"].(string); ok {
				return text
			}
		}
	}
	if items := asItems(content); len(items) > 0 {
		return items
	}
	return content
}

// updatePreviousMemories 更新远期记忆摘要
func updatePreviousMemories() {
	unreadMu.Lock()
	records := append([]PostMessage(nil), unreadRecords...)
	pending := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()
	if len(records) >= 1 && memorizerRole != nil {
		memorizerRole.persistUnreadRecords()
	}
	userMessages := make([]string, 0)
	for _, message := range pending {
		if message.Role != RoleUser {
			continue
		}
		text := strings.TrimSpace(messageText(message))
		if text != "" {
			userMessages = append(userMessages, text)
		}
	}
	if len(userMessages) == 0 {
		if dialogueRole != nil {
			dialogueRole.RagMessages = nil
		}
		return
	}
	digest := ""
	if memorizerRole != nil {
		digest = memorizerRole.queryRagDigest(userMessages)
	}
	if digest == "" {
		if dialogueRole != nil {
			dialogueRole.RagMessages = nil
		}
		return
	}
	if dialogueRole != nil {
		dialogueRole.RagMessages = []PostMessage{{Role: RoleUser, Content: "【长期记忆摘要】\n" + digest}}
	}
}

// normalizeToolImage 把工具返回的图片统一转换为视觉读取引用
//
// 工具（截图者等）天然产出 base64 data URI，尺寸可能大于视觉上限；直接放进
// 上下文会被出站闸门拦截。这里先入库拿句柄，再按思维链形态（视觉读取端点）
// 生成引用，模型看到的永远是受约束的副本。
func normalizeToolImage(image string) string {
	if image == "" {
		return ""
	}
	if handle := handleFromVisionURL(image); handle != "" {
		return image
	}
	admitted := admitImageDataURI(image, imageNameFor(image, resource.KindImage, ""), resource.KindImage)
	if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
		logger.Warn(lunarCoreModule, "[工具调用] 工具图片入库失败，已丢弃该图")
		return ""
	}
	return visionReference(admitted.Handle, admitted.BlobPath)
}

// joinAckParts 按固定顺序（开场 → 动作 → 收尾）丢掉空段，用连接词拼接，再偶尔补一个句尾语气。
// 不做乱序：三段各司其职，位置固定后每次拼装都可预期，读起来也不会前言不搭后语。
func joinAckParts(parts []string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	lastRune := func(value string) rune {
		runes := []rune(value)
		return runes[len(runes)-1]
	}
	joiner := func(current string) string {
		if strings.ContainsRune("，。！？～、", lastRune(current)) {
			return ""
		}
		return pickAckText([]string{"，", "，", "，", "，", "～", " ", ""})
	}
	text := kept[0]
	for _, part := range kept[1:] {
		text += joiner(text) + part
	}
	// 结尾已带语气词（哈/啦/呀…）就不再补，避免拼出「稍等哈哈」这种重复
	if !strings.ContainsRune("，。！？～、哈啦呀呢哦嘛吧", lastRune(text)) {
		text += pickAckText([]string{"", "", "", "", "", "～", "。", "哈"})
	}
	return text
}

// pickAckText 从候选集中随机取一条，空集合返回空串
func pickAckText(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[randomFloor(0, len(items)-1)]
}

// synthesizeSpeechImpl 语音合成实现（前端不在线时返回空串）
func synthesizeSpeechImpl(text string) string {
	if !frontendOnline() {
		return ""
	}
	audio, err := synthesizeText(text)
	if err != nil {
		logger.Warn(lunarCoreModule, "TTS合成失败: %v", err)
		return ""
	}
	return audio
}
