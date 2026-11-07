package thinking

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// newDialogueRole 创建对话者角色
func newDialogueRole(lunarAppearance, crystalAppearance string) *DialogueRole {
	builder := newModelBuilder(Dialogue.SystemPrompt(lunarAppearance, crystalAppearance), Dialogue.Role, 48)
	return &DialogueRole{
		ModelBuilder:      builder,
		LunarAppearance:   lunarAppearance,
		CrystalAppearance: crystalAppearance,
	}
}

// generateDialogue 生成聊天对话（一次应答周期的入口）
func (role *DialogueRole) generateDialogue(cache *ChatCache) {
	role.applyAppendSettings()
	stateMu.Lock()
	lastFailure = ""
	stateMu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error(lunarCoreModule, "["+Dialogue.Role+"] 应答周期异常: %v", recovered)
			stateMu.Lock()
			lastFailure = fmt.Sprintf("内部异常: %v", recovered)
			stateMu.Unlock()
		}
	}()
	role.runToolLoop(cache)
}

// speakAssistantText 推送模型「边说边调工具」时产出的正文。
//
// 这段文本是模型真实说出的话，不是按台词池拼装的快速应答：走 TTS 合成语音。
// auto=true 只表示「这是本轮尚未收尾的旁白」——QQ 桥接据此不置 responseStarted、
// 不消费群 @ 回执与引用候选，把一次性份额留给终稿；否则中间正文一推送就会提前
// 释放应答份额，队列里的下一个请求可能顶替 currentTarget，终稿就会投错会话。
func (role *DialogueRole) speakAssistantText(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	audio := synthesizeSpeech(cleanTextForTTS(content))
	assistantTextPushHook("text", content, audio, true)
}

func (role *DialogueRole) fail(err error) {
	reason := err.Error()
	logger.Error(lunarCoreModule, "["+Dialogue.Role+"] 请求处理错误: %s", reason)
	stateMu.Lock()
	lastFailure = reason
	stateMu.Unlock()
	reportAgentFailure(reason)
}

// formatHistoricalMessages 格式化历史消息
func (role *DialogueRole) formatHistoricalMessages() {
	if len(role.Messages) == 0 {
		return
	}
	lastRole := role.Messages[len(role.Messages)-1].Role
	if lastRole == RoleUser || lastRole == RoleTool {
		return
	}
	role.writeContext(PostMessage{Role: RoleUser, Content: "继续"})
}

// applyAppendSettings 把追加设定拼到系统提示词末尾
func (role *DialogueRole) applyAppendSettings() {
	lines := Dialogue.ReadAppendSettings()
	// 这里重建的是**原始模板**，必须再过一次占位符补全：
	// 构造时补全过的 systemPrompt 会在每轮被这里覆盖，漏掉补全就会把
	// 「当前位置：{current-address}」原样交给模型（表现为月华不知道自己在哪座城市）。
	base := role.promptCompletion(Dialogue.SystemPrompt(role.LunarAppearance, role.CrystalAppearance))
	if len(lines) == 0 {
		role.SystemPrompt = base
		return
	}
	role.SystemPrompt = strings.TrimRight(base, " \t\r\n") + "\n" + strings.Join(lines, "\n")
}

// analyzeModelResponse 处理模型响应
func analyzeModelResponse(body ModelResponseBody, cache *ChatCache) {
	if len(body.Choices) == 0 {
		return
	}
	message := body.Choices[0].Message
	if message.Content != "" {
		cache.DescriptionContent = message.Content
	}
	for _, call := range message.ToolCalls {
		raw := strings.TrimSpace(call.Function.Arguments)
		if raw != "" && raw != "{}" {
			var parsed map[string]any
			if json.Unmarshal([]byte(raw), &parsed) != nil {
				logger.Warn(lunarCoreModule, "["+Dialogue.Role+"] 工具调用参数解析失败，已跳过: %s", call.Function.Name)
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
