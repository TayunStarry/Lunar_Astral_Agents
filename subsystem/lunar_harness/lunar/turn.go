package lunar

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/BilibiliLife"
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

func slowTaskAckText(functionName string, args map[string]any) string {
	leads := []string{"收到", "好嘞", "交给我吧", "没问题", "这个我来"}
	ends := []string{"～", "，稍等我一下～", "，等我好消息～", "，一会儿就好～"}
	pick := func(items []string) string { return items[randomFloor(0, len(items)-1)] }
	compose := func(actions []string) string {
		return pick(leads) + "！" + pick(actions) + pick(ends)
	}
	if functionName == "dispatch_painter" {
		return compose([]string{"这就动笔", "这就开画", "马上开工", "先起个稿"})
	}
	if functionName == crystalToolName {
		if strings.ToLower(strings.TrimSpace(stringArg(args, "tool"))) == "web_search" {
			return compose([]string{"这就去翻资料", "这就去查", "马上去找", "先去翻一翻"})
		}
	}
	return ""
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

// lifeProgress 生活链当前进度
func lifeProgress() string {
	return BilibiliLife.Progress()
}

func lifeAutoTick() {
	BilibiliLife.AutoTick()
}

func lifeSetBusy(busy bool) {
	BilibiliLife.SetDialogueBusy(busy)
}

func lifeDialogueIdle() {
	BilibiliLife.NotifyDialogueIdle()
}
