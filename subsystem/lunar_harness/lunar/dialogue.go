package lunar

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

var actionClaimPattern = regexp.MustCompile(`(已经|已|这就|马上|现在)[^，。！？\n]{0,12}(打开|启动|运行|调出|叫出|切换|准备好|准备完毕)`)

const actionClaimGuard = "【系统校验】你刚才声称已经完成了打开/启动类操作，但本轮没有调用任何工具，用户看不到任何变化。请直接调用 use_the_program 执行（只打开应用只填 tool），或如实说明没有完成，不要只口头宣称。"

// newDialogueRole 创建对话者角色
func newDialogueRole(lunarAppearance, crystalAppearance string) *DialogueRole {
	builder := newModelBuilder(composeDialoguePrompt(lunarAppearance, crystalAppearance), "对话者", 48)
	return &DialogueRole{
		ModelBuilder:      builder,
		LunarAppearance:   lunarAppearance,
		CrystalAppearance: crystalAppearance,
		ImageWindow:       imageWindowSize,
	}
}

// generateDialogue 生成聊天对话（一次应答周期的入口）
func (role *DialogueRole) generateDialogue(cache *ChatCache) {
	role.applyAppendSettings()
	role.ToolRounds = 0
	role.ActionClaimGuarded = false
	role.PendingSpeeches = nil
	stateMu.Lock()
	lastFailure = ""
	stateMu.Unlock()
	role.continueDialogue(cache, nil)
}

// continueDialogue 应答周期内部的一步：请求模型 → 工具调用 → 递归
func (role *DialogueRole) continueDialogue(cache *ChatCache, guardNote *PostMessage) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error(lunarCoreModule, "[对话者] 应答周期异常: %v", recovered)
			stateMu.Lock()
			lastFailure = fmt.Sprintf("内部异常: %v", recovered)
			stateMu.Unlock()
		}
	}()

	LiteImageFile()
	unreadMu.Lock()
	pending := append([]PostMessage(nil), unreadContext...)
	unreadContext = make([]PostMessage, 0)
	unreadMu.Unlock()
	for _, message := range pending {
		role.writeContext(message)
	}
	role.formatHistoricalMessages()
	role.RuntimeMessages = []PostMessage{{Role: RoleUser, Content: "当前时间: " + time.Now().Format("2006-01-02 15:04:05")}}

	contextMessages := role.RagMessages
	if guardNote != nil {
		contextMessages = append(append([]PostMessage{}, role.RagMessages...), *guardNote)
	}
	tools := append([]ToolCall{}, agentControlTools...)
	tools = append(tools, memoryTools...)
	tools = append(tools, resourceTools...)
	tools = append(tools, scheduleTools...)
	tools = append(tools, networkExploreToolCalls()...)
	tools = append(tools, getCrystalRemoteTools()...)

	response, err := role.run(contextMessages, tools)
	if err != nil {
		role.fail(err)
		return
	}
	if len(response.Body.Error) > 0 && strings.TrimSpace(string(response.Body.Error)) != "null" {
		role.fail(fmt.Errorf("模型服务错误: %s", string(response.Body.Error)))
		return
	}
	if len(response.Body.Choices) == 0 {
		role.fail(fmt.Errorf("模型响应缺少 choices"))
		return
	}
	analyzeModelResponse(response.Body, cache)
	message := response.Body.Choices[0].Message
	content := strings.TrimSpace(message.Content)

	if len(cache.ToolCalls) > 0 {
		role.writeContext(messageFromAssistant(message))
		if role.ToolRounds >= maxToolRounds {
			logger.Warn(lunarCoreModule, "[对话者] 工具调用轮次达到上限（%d），停止继续调用", maxToolRounds)
			// 本轮随工具调用说出的正文同样暂存，交给终稿一起输出
			role.rememberPendingSpeech(content)
			role.rejectPendingToolCalls(cache)
			role.finalizeWithoutTools()
			return
		}
		if role.batchExecutionToolCall(cache) {
			// 模型一边说话一边调用工具：这段正文先按时间顺序暂存，
			// 等工具回调后的正式回答出现时再拼到前面，避免被下一轮响应覆盖而丢失。
			role.rememberPendingSpeech(content)
			role.ToolRounds++
			role.continueDialogue(cache, nil)
			return
		}
	}

	if !role.ActionClaimGuarded && len(cache.ToolCalls) == 0 && role.ToolRounds == 0 && actionClaimPattern.MatchString(cache.DescriptionContent) {
		role.ActionClaimGuarded = true
		logger.Warn(lunarCoreModule, "[对话者] 未调用工具却宣称已完成动作，触发纠偏重答")
		cache.DescriptionContent = ""
		cache.ThinkingContent = ""
		role.continueDialogue(cache, &PostMessage{Role: RoleUser, Content: actionClaimGuard})
		return
	}

	role.writeContext(messageFromAssistant(message))
	notifyAgentSuccess()
	final := role.composeFinalSpeech(content)
	stateMu.Lock()
	finalResponse = final
	stateMu.Unlock()
}

// rememberPendingSpeech 暂存「边说边调工具」时随工具调用输出的正文
func (role *DialogueRole) rememberPendingSpeech(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	role.PendingSpeeches = append(role.PendingSpeeches, content)
}

// composeFinalSpeech 把暂存的途中正文按时间顺序拼到正式回答前面，并清空暂存
func (role *DialogueRole) composeFinalSpeech(final string) string {
	parts := make([]string, 0, len(role.PendingSpeeches)+1)
	parts = append(parts, role.PendingSpeeches...)
	if final = strings.TrimSpace(final); final != "" {
		parts = append(parts, final)
	}
	role.PendingSpeeches = nil
	return strings.Join(parts, "\n\n")
}

func (role *DialogueRole) fail(err error) {
	reason := err.Error()
	logger.Error(lunarCoreModule, "[对话者] 请求处理错误: %s", reason)
	stateMu.Lock()
	lastFailure = reason
	stateMu.Unlock()
	reportAgentFailure(reason)
}

// rejectPendingToolCalls 回填未执行工具结果
func (role *DialogueRole) rejectPendingToolCalls(cache *ChatCache) {
	reason := fmt.Sprintf("本轮工具调用已达上限（%d 次），该调用未执行", maxToolRounds)
	for _, call := range cache.ToolCalls {
		role.Messages = append(role.Messages, PostMessage{Role: RoleTool, Content: reason, ToolCallID: call.ID})
	}
	cache.ToolCalls = nil
}

// finalizeWithoutTools 不带工具再要一次终稿
func (role *DialogueRole) finalizeWithoutTools() {
	response, err := role.run(role.RagMessages, nil)
	if err == nil && len(response.Body.Choices) > 0 {
		content := strings.TrimSpace(response.Body.Choices[0].Message.Content)
		if content != "" {
			stateMu.Lock()
			finalResponse = role.composeFinalSpeech(content)
			stateMu.Unlock()
			role.writeContext(PostMessage{Role: RoleAssistant, Content: content})
			return
		}
	}
	stateMu.Lock()
	finalResponse = role.composeFinalSpeech("这轮步骤有点多了，我先停在这里，你可以接着往下说。")
	stateMu.Unlock()
}

// formatHistoricalMessages 格式化历史消息
func (role *DialogueRole) formatHistoricalMessages() {
	if len(role.Messages) == 0 {
		return
	}
	role.demoteOverflowImages()
	if len(role.Messages) == 0 {
		return
	}
	lastRole := role.Messages[len(role.Messages)-1].Role
	if lastRole == RoleUser || lastRole == RoleTool {
		return
	}
	role.writeContext(PostMessage{Role: RoleUser, Content: "继续"})
}

// demoteOverflowImages 增量降级超窗口图片
func (role *DialogueRole) demoteOverflowImages() {
	countImages := func() int {
		count := 0
		for _, message := range role.Messages {
			for _, item := range asItems(message.Content) {
				if item.Type == ContentImageURL {
					count++
				}
			}
		}
		return count
	}
	for i := 0; i < len(role.Messages) && countImages() > role.ImageWindow; i++ {
		items := asItems(role.Messages[i].Content)
		if len(items) == 0 {
			continue
		}
		hasImage := false
		for _, item := range items {
			if item.Type == ContentImageURL {
				hasImage = true
				break
			}
		}
		if !hasImage {
			continue
		}
		rebuilt := make([]ContentItem, 0, len(items))
		for _, item := range items {
			if item.Type != ContentImageURL || item.ImageURL == nil {
				rebuilt = append(rebuilt, item)
				continue
			}
			tag := handleFromVisionURL(item.ImageURL.URL)
			if tag == "" {
				rebuilt = append(rebuilt, ContentItem{Type: ContentText, Text: "（一张未能登记句柄的图片，已超出窗口）"})
				continue
			}
			rebuilt = append(rebuilt, ContentItem{Type: ContentText, Text: composeImagePlaceholder(tag, role.describeImage(tag, item.ImageURL.URL))})
		}
		role.Messages[i].Content = rebuilt
	}
}

// describeImage 获取图片条目描述
func (role *DialogueRole) describeImage(tag, _ string) string {
	if entry := getResourceEntry(tag); entry != nil && entry.Summary != "" {
		return sanitizeImageDesc(entry.Summary, 20)
	}
	return sanitizeImageDesc(describeResource(tag, ""), 20)
}

// applyAppendSettings 把追加设定拼到系统提示词末尾
func (role *DialogueRole) applyAppendSettings() {
	lines := readAppendSettings()
	// 这里重建的是**原始模板**，必须再过一次占位符补全：
	// 构造时补全过的 systemPrompt 会在每轮被这里覆盖，漏掉补全就会把
	// 「当前位置：{current-address}」原样交给模型（表现为月华不知道自己在哪座城市）。
	base := role.promptCompletion(composeDialoguePrompt(role.LunarAppearance, role.CrystalAppearance))
	if len(lines) == 0 {
		role.SystemPrompt = base
		return
	}
	role.SystemPrompt = strings.TrimRight(base, " \t\r\n") + "\n" + strings.Join(lines, "\n")
}

// batchExecutionToolCall 批量执行工具调用
func (role *DialogueRole) batchExecutionToolCall(cache *ChatCache) bool {
	return executeToolCalls(&role.Messages, cache)
}

// analyzeModelResponse 处理模型响应
func analyzeModelResponse(body ModelResponseBody, cache *ChatCache) {
	if len(body.Choices) == 0 {
		return
	}
	message := body.Choices[0].Message
	if message.ReasoningContent != "" {
		cache.ThinkingContent = message.ReasoningContent
	}
	if body.Timings != nil && body.Timings.PredictedPerSecond > 0 {
		logger.Info(lunarCoreModule, "词元生成速度: %.2f", body.Timings.PredictedPerSecond)
	}
	if message.Content != "" {
		cache.DescriptionContent = message.Content
	}
	for _, call := range message.ToolCalls {
		raw := strings.TrimSpace(call.Function.Arguments)
		if raw != "" && raw != "{}" {
			var parsed map[string]any
			if json.Unmarshal([]byte(raw), &parsed) != nil {
				logger.Warn(lunarCoreModule, "[对话者] 工具调用参数解析失败，已跳过: %s", call.Function.Name)
				continue
			}
		}
		cache.ToolCalls = append(cache.ToolCalls, call)
	}
}

// messageFromAssistant 把助手消息转成上下文消息
func messageFromAssistant(message AssistantMessage) PostMessage {
	result := PostMessage{Role: RoleAssistant, Content: message.Content}
	if len(message.ToolCalls) > 0 {
		result.ToolCalls = message.ToolCalls
	}
	return result
}
