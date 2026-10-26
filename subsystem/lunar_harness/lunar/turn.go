package lunar

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// writeMessage 写入消息（文本或多模态内容）
func writeMessage(role string, content any) {
	normalized := normalizeIncomingContent(content)
	message := PostMessage{Role: role, Content: normalized}
	attachImageTags(&message)
	if role == RoleUser {
		appendUserMessage(message.Content)
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
				logger.Info(lunarCoreModule, "收到图片: %s", truncateRunes(item.ImageURL.URL, 50))
			}
		case ContentInputAudio:
			if item.InputAudio != nil {
				logger.Info(lunarCoreModule, "收到音频: %s", truncateRunes(item.InputAudio.Data, 30))
			}
		case ContentVideoURL:
			if item.VideoURL != nil {
				logger.Info(lunarCoreModule, "收到视频: %s", item.VideoURL.URL)
			}
		case ContentAudioURL:
			if item.AudioURL != nil {
				logger.Info(lunarCoreModule, "收到音频: %s", item.AudioURL.URL)
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

// guardASRUnload 本轮无语音时卸载 ASR 模型
func guardASRUnload() {
	unreadMu.Lock()
	messages := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()
	hasAudio := false
	for _, message := range messages {
		for _, item := range asItems(message.Content) {
			if item.Type == ContentInputAudio || item.Type == ContentAudioURL {
				hasAudio = true
				break
			}
		}
	}
	if hasAudio || len(messages) == 0 {
		return
	}
	body := map[string]any{"model": config.AgentASRModel}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, config.LocalBaseURL()+"/vram/unload", bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn(lunarCoreModule, "ASR 卸载执行失败: %v", err)
		return
	}
	resp.Body.Close()
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

// executeToolCalls 批量执行工具调用
func executeToolCalls(messages *[]PostMessage, cache *ChatCache) bool {
	hasToolCalls := false
	for _, call := range cache.ToolCalls {
		name := call.Function.Name
		handler := lookupTool(name)
		if handler == nil {
			*messages = append(*messages, PostMessage{Role: RoleTool, Content: "未找到工具包: " + name, ToolCallID: call.ID})
			continue
		}
		args := parseToolArgs(call.Function.Arguments)
		maybeSendFastAck(name, args)
		result := handler(args)
		*messages = append(*messages, PostMessage{Role: RoleTool, Content: result.Text, ToolCallID: call.ID})
		if image := normalizeToolImage(result.Image); image != "" {
			imageMessage := PostMessage{
				Role: RoleUser,
				Content: []ContentItem{
					{Type: ContentText, Text: "【工具返回的图片】" + result.Text},
					{Type: ContentImageURL, ImageURL: &URLPayload{URL: image}},
				},
			}
			attachImageTags(&imageMessage)
			*messages = append(*messages, imageMessage)
			logger.Info(lunarCoreModule, "[工具调用] %s 返回图片数据，长度=%d 字节", name, len(result.Image))
		}
		hasToolCalls = true
	}
	cache.CurrentToolCallIndex = -1
	cache.CurrentFunctionArgs = ""
	cache.CurrentFunctionName = ""
	cache.CurrentToolCall = nil
	cache.ToolCalls = nil
	return hasToolCalls
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
	admitted := admitImageDataURI(image, "image"+guessImageExt(image))
	if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
		logger.Warn(lunarCoreModule, "[工具调用] 工具图片入库失败，已丢弃该图")
		return ""
	}
	return visionReference(admitted.Handle, admitted.BlobPath)
}

// performMatchedAction 执行匹配到的动作
func performMatchedAction(query string) {
	bestAction := matchActionName(strings.TrimSpace(query))
	if bestAction == "" {
		return
	}
	position := currentAgentPosition()
	payload := map[string]any{
		"position": map[string]any{
			"x": position.X + randomFloat(-0.5, 0.5),
			"y": position.Y,
			"z": position.Z + randomFloat(-0.5, 0.5),
		},
		"resumeTracking": true,
	}
	sendToEngine("movement", safeJSON(payload, "{}"))
	sendToEngine("action", safeJSON(map[string]any{"action": bestAction}, "{}"))
	pushAssistantContext("action", "月华"+bestAction+"了", "", false)
}

// maybeSendFastAck 高耗时任务的快速应答
func maybeSendFastAck(functionName string, args map[string]any) {
	stateMu.Lock()
	if fastAckSent {
		stateMu.Unlock()
		return
	}
	stateMu.Unlock()
	text := slowTaskAckText(functionName, args)
	if text == "" {
		return
	}
	stateMu.Lock()
	fastAckSent = true
	fromQQ := turnFromQQ
	stateMu.Unlock()
	if audio := synthesizeSpeech(text); audio != "" {
		pushAssistantContext("text", text, audio, fromQQ)
		return
	}
	pushAssistantContext("text", text, "", fromQQ)
}

// slowTaskAckText 高耗时任务的口语化快速应答
//
// 应答由「开头 / 正文 / 结尾」三段随机拼装：每段都可能缺席，三段的先后顺序也随机，
// 段间连接词与句尾语气再各自随机，尽量不让固定句式被风控识别成机器人。
func slowTaskAckText(functionName string, args map[string]any) string {
	actions := slowAckActions(functionName, args)
	if len(actions) == 0 {
		return ""
	}
	openings := []string{
		"", "", "", // 有时没有开头，直接说正文
		"收到", "收到收到", "好嘞", "好嘞好嘞", "交给我吧", "这个交给我", "没问题",
		"没问题哈", "这个我来", "我来我来", "嗯嗯", "嗯呢", "好的", "好哒", "好呀",
		"行", "可以呀", "知道啦", "知道知道", "明白", "明白啦", "来啦", "来咯",
		"我看看", "这就安排", "马上安排", "安排上了", "OK", "OK啦", "这个简单",
		"放心交给我", "包在我身上", "我来接手", "我这边来", "好说好说", "哦哦",
		"嗯", "诶好", "可以可以", "我这就弄",
	}
	endings := []string{
		"", "", "", // 有时没有结尾，说完正文就收
		"稍等我一下", "稍等哈", "稍等片刻", "稍等哦", "等我一下下", "等我一会儿",
		"等我下哈", "等我下下", "等我好消息", "一会儿就好", "一会儿见", "马上回来",
		"马上就来", "马上搞定", "马上就好", "马上给你结果", "很快的", "很快哦",
		"很快搞定", "很快回来", "很快就有结果", "这就去办", "这就给你办",
		"不耽误你太久", "给我点时间", "我尽快", "别急哈", "耐心等我一小会儿",
		"这边很快就完事", "一下下就好", "马上哈", "稍等马上来",
	}
	text := joinAckParts([]string{
		pickAckText(openings),
		pickAckText(actions),
		pickAckText(endings),
	})
	if text == "" {
		for _, action := range actions {
			if action != "" {
				text = action
				break
			}
		}
	}
	return text
}

// slowAckActions 按工具给出正文候选；空串表示这次不说正文，返回 nil 表示该工具不需要快速应答
func slowAckActions(functionName string, args map[string]any) []string {
	switch functionName {
	case "dispatch_painter":
		return []string{
			"这就动笔", "这就开画", "马上开工", "先起个稿", "我先构图", "我调下画笔",
			"先铺个底色", "这就安排画面", "先把色稿打出来", "马上开始画", "让我先想想构图",
			"我这就画起来", "这就给你画一张", "先把线稿理出来", "我找找灵感就开工",
			"先试试笔触", "我这就动手", "开始画啦", "先把草稿起一下", "马上给你出图",
			"这就铺色", "先把轮廓勾出来", "马上落笔", "先画个大概", "", "",
		}
	case networkExploreToolName:
		switch strings.ToLower(strings.TrimSpace(stringArg(args, "tool"))) {
		case networkWebSearchTool:
			return []string{
				"这就去翻资料", "这就去查", "马上去找", "先去翻一翻", "我去搜一圈",
				"这就帮你搜", "我查查看", "让我去扒一下资料", "马上把资料找回来",
				"我去网上问问", "这就去找找看", "我先搜一遍", "这题我查查", "我这就去搜",
				"让我翻翻看", "去检索一下", "先看看网上怎么说", "这就去核实一下",
				"我先去检索", "马上收集资料", "去查查最新消息", "我上网翻翻", "", "",
			}
		case networkBilibilViewerTool:
			return []string{
				"这就去看会儿", "那我去逛逛B站", "我去刷一会儿", "这就去挑个视频",
				"我去B站看看", "这就打开B站", "我去刷两个", "让我先刷刷看", "这就去逛一圈",
				"去看看有没有好玩的", "马上开刷", "我去B站转一圈", "这就给你挑", "我去瞅瞅",
				"我这就去刷", "去B站找找乐子", "先随便刷两下", "去看看热门", "我去翻翻推荐",
				"找个视频看看", "", "",
			}
		case networkQQSendTool:
			return []string{
				"这就去发", "马上把消息送过去", "这就帮你转达", "稍等我去发一下",
				"我这就发出去", "马上发", "这就发给你", "我去发送一下", "我这就把话带到",
				"这就去发消息", "马上送达", "我这就传过去", "这就发过去", "马上去发",
				"我这就去传达", "帮你传达一下", "这就把消息发出去", "我去联系一下",
				"马上转告", "", "",
			}
		}
	}
	return nil
}

// joinAckParts 丢掉缺席的段落，随机打乱顺序后用随机连接词拼接，再偶尔补一个句尾语气
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
		return pickAckText([]string{"，", "，", "，", "～", "！", "。", "", ""})
	}
	rand.Shuffle(len(kept), func(i, j int) { kept[i], kept[j] = kept[j], kept[i] })
	text := kept[0]
	for _, part := range kept[1:] {
		text += joiner(text) + part
	}
	if !strings.ContainsRune("，。！？～、", lastRune(text)) {
		text += pickAckText([]string{"", "", "", "～", "～", "！", "。", "呀～", "哈～", "啦～", "哦～"})
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

// synthesizeSpeech 语音合成（前端不在线时返回空串）
func synthesizeSpeech(text string) string {
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
