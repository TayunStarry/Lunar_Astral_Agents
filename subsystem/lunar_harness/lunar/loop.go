package lunar

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/engine/affection"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/life"
	"LunarSubsystem/LunarHarness/engine/mood"
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

const failureReply = "刚才有点走神，你再说一遍好吗？"

// Start 启动 Go 原生月华智能体
func Start() error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if running.Load() {
		return nil
	}
	initSchedules()
	registerDialogueTools()
	syncLTPXRemoteTools()

	// 角色实例：绘制者先解析角色提示词，对话者复用其基础外观
	painterRole = newPainterRole()
	musicianRole = newMusicianRole()
	screenshoterRole = newScreenshoterRole()
	dialogueRole = newDialogueRole(painterRole.lunarAppearance(), painterRole.crystalAppearance())
	memorizerRole = &MemorizerRole{}

	initConversationCache(func(messages []PostMessage) {
		dialogueRole.coverContext(messages)
	})

	life.Init()
	life.SetPlanHooks(pushDueScheduleItems, upcomingPlanSummary)
	life.SetLastUserTextProvider(latestUserUtterance)
	life.SetLastUserNameProvider(latestUserName)
	life.SetLastReplyTextProvider(latestAssistantUtterance)
	go func() {
		if err := mood.Init(context.Background()); err != nil {
			logger.Warn(lunarCoreModule, "[心情] 启动初始化未完成，将在生活事件中重试: %v", err)
		}
		if err := affection.Init(context.Background()); err != nil {
			logger.Warn(lunarCoreModule, "[情感] 启动初始化未完成，将在生活事件中重试: %v", err)
		}
	}()

	tickSchedulerInstance = buildTickScheduler()

	agentReady.Store(true)
	stopCh = make(chan struct{})
	running.Store(true)
	loopWG.Add(1)
	go runLoop()
	logger.Info(lunarCoreModule, "月华智能体（统一 Tick 调度）已启动")
	return nil
}

// Stop 停止循环
func Stop() {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !running.Load() {
		return
	}
	close(stopCh)
	loopWG.Wait()
	if tickSchedulerInstance != nil {
		tickSchedulerInstance.Stop()
		tickSchedulerInstance = nil
	}
	running.Store(false)
	agentReady.Store(false)
}

// IsReady 报告智能体是否已就绪
func IsReady() bool {
	return agentReady.Load()
}

func buildTickScheduler() *tickScheduler {
	scheduler := newTickScheduler()
	scheduler.Register(dialogueSystem{})
	scheduler.Register(viewingSystem{})
	scheduler.Register(lifeSystem{})
	return scheduler
}

func runLoop() {
	defer loopWG.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			scheduler := tickSchedulerInstance
			if scheduler != nil {
				scheduler.Tick()
			}
		}
	}
}

// pullContext 拉取统一未读队列
func pullContext() []PostMessage {
	messages := TakeUnreadContext()
	if len(messages) == 0 {
		DialogueBridging.NotifyAgentIdle()
		return nil
	}
	messages = dispatchBeforeReadingMessage(messages)
	if len(messages) == 0 {
		DialogueBridging.NotifyAgentIdle()
		return nil
	}
	// 进入上下文前统一资源化：图片→缩影链接；动图/视频/音频/URL/BV→句柄文本；
	// 文件→句柄。资源在接入阶段已完成下载、校验与句柄绑定，解析留到 use_resource。
	return ResourcesConversionHandle(messages)
}

// dispatchBeforeReadingMessage 读取消息前事件
func dispatchBeforeReadingMessage(messages []PostMessage) []PostMessage {
	if len(messages) == 0 {
		return messages
	}
	result := StarLTP.Emit(StarLTP.EventBeforeReadingMessage, map[string]any{"messages": messages}, "")
	feedback, ok := result.Return.(map[string]any)
	if !result.Returned || !ok {
		return messages
	}
	raw, has := feedback["messages"]
	if !has {
		return messages
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return messages
	}
	var out []PostMessage
	if json.Unmarshal(data, &out) != nil {
		return messages
	}
	return out
}

// runDialogueTurn 对话链一次完整回合
func runDialogueTurn(incoming []PostMessage) {
	// 对话者即将推理：先终止外部桌面智能体（桌面操纵），避免它继续操作桌面干扰本轮响应
	stopDesktopActor()
	// 本轮情感标识只跟随本轮到达的用户消息，先清掉上一轮遗留。
	resetEmotionMarks()

	stateMu.Lock()
	fastAckSent = false
	turnFromQQ = false
	stateMu.Unlock()
	defer persistConversation()

	for _, message := range incoming {
		if message.Source == "qq" {
			stateMu.Lock()
			turnFromQQ = true
			stateMu.Unlock()
		}
		writeMessage(message.Role, message.Source, message.Content)
	}

	unreadMu.Lock()
	messageLength := len(unreadContext)
	unreadMu.Unlock()
	if messageLength == 0 {
		return
	}
	updatePreviousMemories()

	cache := &ChatCache{CurrentToolCallIndex: -1}
	dialogueRole.generateDialogue(cache)
	stateMu.Lock()
	finalText := finalResponse
	failure := lastFailure
	stateMu.Unlock()

	if strings.TrimSpace(finalText) == "" {
		if failure != "" {
			pushAssistantContext("text", failureReply, "", false)
			logger.Warn(lunarCoreModule, "[思考链] 本轮应答失败: %s", failure)
			return
		}
		defaultMessage := randomDefaultMessage()
		pushAssistantContext("text", defaultMessage, synthesizeSpeech(defaultMessage), false)
		return
	}

	parsed := parseContent(finalText)
	validMessage := make([]string, 0, len(parsed.TextChunks))
	for _, chunk := range parsed.TextChunks {
		validMessage = append(validMessage, chunk.Display)
	}
	cleaned := strings.TrimSpace(strings.Join(validMessage, "\n"))
	if cleaned == "" {
		logger.Error(lunarCoreModule, "[对话链] 清洗后的文本为空")
		pushAssistantContext("text", randomDefaultMessage(), "", false)
		return
	}
	replyText := strings.TrimSpace(parsed.DisplayText)
	if replyText == "" {
		replyText = cleaned
	}

	if len(parsed.ActionBlocks) > 0 {
		query := strings.TrimSpace(strings.Join(parsed.ActionBlocks, " "))
		if query == "" {
			query = cleaned
		}
		performMatchedAction(query)
		if sticker := queryEmotionSticker(cleaned); sticker != "" {
			pushAssistantImage([]string{sticker}, true)
		}
	} else if len([]rune(cleaned)) <= 36 && randomFloat(0, 1) < 0.15 {
		if sticker := queryEmotionSticker(cleaned); sticker != "" {
			pushAssistantImage([]string{sticker}, true)
		}
	}

	for _, thinking := range parsed.ThinkingBlocks {
		pushAssistantContext("text", thinking, "", false)
	}
	for _, code := range parsed.CodeBlocks {
		pushAssistantContext("text", code, "", false)
	}
	audio := synthesizeSpeech(cleanTextForTTS(replyText))
	pushAssistantContext("text", replyText, audio, false)
}

// randomDefaultMessage 随机回答
func randomDefaultMessage() string {
	messages := []string{"月华在哦", "怎么了吗?", "详细说说?"}
	return messages[randomFloor(0, len(messages)-1)]
}

// ==== 队列入口 ====

// EnqueueMessage 将消息按到达顺序追加到统一未读队列
func EnqueueMessage(messages ...PostMessage) {
	unreadMu.Lock()
	unreadContext = append(unreadContext, messages...)
	unreadMu.Unlock()
}

// EnqueueAudioURL 将语音/音频 URL 包装为音频内容项并追加到队列（入库与转写出智能体侧完成）
func EnqueueAudioURL(url, source string) {
	EnqueueMessage(PostMessage{
		Role:    RoleUser,
		Content: []ContentItem{{Type: ContentAudioURL, AudioURL: &URLPayload{URL: url}}},
		Source:  source,
	})
}

// TakeUnreadContext 拉取并清空统一未读队列
func TakeUnreadContext() []PostMessage {
	unreadMu.Lock()
	defer unreadMu.Unlock()
	if len(unreadContext) == 0 {
		return nil
	}
	messages := append([]PostMessage{}, unreadContext...)
	unreadContext = make([]PostMessage, 0)
	return messages
}

// UnreadCount 返回未读队列长度
func UnreadCount() int {
	unreadMu.Lock()
	defer unreadMu.Unlock()
	return len(unreadContext)
}
