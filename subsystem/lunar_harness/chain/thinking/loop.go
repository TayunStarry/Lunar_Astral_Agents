package thinking

import (
	"context"
	"errors"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
)

const failureReply = "刚才有点走神，你再说一遍好吗？"

// Start 启动 Go 原生月华智能体
func Start() error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if running.Load() {
		return nil
	}
	initWishes()
	Dialogue.SyncLTPXRemoteTools()

	// 角色实例：绘制者外观由扁平智能体提供，对话者复用其基础外观
	lunarAppearance, crystalAppearance := Painter.Appearances()
	dialogueRole = newDialogueRole(lunarAppearance, crystalAppearance)
	memorizerRole = &MemorizerRole{}

	initConversationCache(func(messages []PostMessage) {
		dialogueRole.coverContext(messages)
	})

	life.Init()
	// 演绎任务现场落盘后，启动时恢复上次进程遗留的未完成任务：
	// 能还原现场的从原阶段继续，无法还原的标记中断并推送原因。
	storyboard.RecoverInterrupted()
	life.SetWishHooks(pushDueWishItems, upcomingWishSummary)
	life.SetDialogueRoundsProvider(recentAffectionRounds)
	go func() {
		if err := wishchain.Init(context.Background()); err != nil {
			logger.Warn(lunarCoreModule, "[心愿] 启动初始化未完成，将在生活事件中重试: %v", err)
		}
	}()

	tickSchedulerInstance = buildTickScheduler()
	// 观影是后台活动：用户消息到达时有更高的优先级，观影在安全边界让出，
	// 先把消息回应完，再由后续 Tick 接着看（见 chain/viewer/interrupt.go）。
	viewer.SetPauseHook(func() bool { return UnreadCount() > 0 })

	agentReady.Store(true)
	stopCh = make(chan struct{})
	running.Store(true)
	loopWG.Add(1)
	go runLoop()
	logger.Info(lunarCoreModule, "月华智能体（单 Tick 调度 + 特许响应车道）已启动")
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
	scheduler.Register(logisticsSystem{})
	// 外部响应链：无主链运行时按优先级调度；特许运行期间由调度器单独放行。
	scheduler.Register(responseSystem{})
	// 演绎链：用户发起的后台任务，按原子步骤推进，镜头之间立即让出 Tick。
	scheduler.Register(newStoryboardSystem())
	scheduler.Register(newLifeSystem())
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
	// 进入上下文前统一资源化：图片→缩影链接；动图/视频/音频/URL/BV→句柄文本；
	// 文件→句柄。资源在接入阶段已完成下载、校验与句柄绑定，解析留到 fetch_resources。
	return ResourcesConversionHandle(messages)
}

// runDialogueTurn 思维链一次完整回合
func runDialogueTurn(incoming []PostMessage) {
	// 对话者即将推理：先终止外部桌面智能体（桌面操纵），避免它继续操作桌面干扰本轮响应
	stopDesktopActor()
	// 本轮情感标识只跟随本轮到达的用户消息，先清掉上一轮遗留。
	resetEmotionMarks()

	stateMu.Lock()
	fastAckSent = false
	stateMu.Unlock()
	defer func() {
		persistConversation()
		// 本轮已回应完毕：立即推进 QQ 桥接队列，不必等空闲 Tick。
		// 观影一轮尚未结束时 Tick 几乎不会空闲，排队消息会一直等到整轮结束。
		notifyIdleHook()
	}()

	for _, message := range incoming {
		writeMessage(message.Role, message.Source, message.Content)
	}

	unreadMu.Lock()
	messageLength := len(unreadContext)
	unreadMu.Unlock()
	if messageLength == 0 {
		return
	}
	updatePreviousMemories()

	cache := &ChatCache{}
	dialogueRole.generateDialogue(cache)
	stateMu.Lock()
	finalText := finalResponse
	failure := lastFailure
	stateMu.Unlock()

	if strings.TrimSpace(finalText) == "" {
		if failure != "" {
			// 交给拯救者：上下文类故障（超长、坏图）由后勤体系在后续 Tick 修复
			reportDialogueFailure(errors.New(failure))
			pushAssistantContext("text", failureReply, synthesizeSpeech(failureReply), false)
			logger.Warn(lunarCoreModule, "[思维链] 本轮应答失败: %s", failure)
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
		logger.Error(lunarCoreModule, "[思维链] 清洗后的文本为空")
		fallback := randomDefaultMessage()
		pushAssistantContext("text", fallback, synthesizeSpeech(fallback), false)
		return
	}
	replyText := strings.TrimSpace(parsed.DisplayText)
	if replyText == "" {
		replyText = cleaned
	}

	// 动作由前端引擎根据推送消息自行匹配；思维链只负责表情包
	if len(parsed.ActionBlocks) > 0 {
		if sticker := queryEmotionSticker(cleaned); sticker != "" {
			pushAssistantImage([]string{sticker}, true)
		}
	} else if len([]rune(cleaned)) <= 36 && randomFloat(0, 1) < 0.15 {
		if sticker := queryEmotionSticker(cleaned); sticker != "" {
			pushAssistantImage([]string{sticker}, true)
		}
	}

	pushProcessBlocks(parsed.ThinkingBlocks, parsed.CodeBlocks)
	audio := synthesizeSpeech(cleanTextForTTS(replyText))
	pushAssistantContext("text", replyText, audio, false)
}

// pushProcessBlocks 推送思考区与代码块等过程内容。
//
// 这些块只用于前端展示，不是终稿：auto=true 保证它们既不消费 QQ 的 @ / 引用回执，
// 也不会提前置位「已回应」把排队请求顶上 currentTarget；终稿才负责收尾。
func pushProcessBlocks(thinkingBlocks, codeBlocks []string) {
	for _, thinking := range thinkingBlocks {
		pushAssistantContext("text", thinking, "", true)
	}
	for _, code := range codeBlocks {
		pushAssistantContext("text", code, "", true)
	}
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

// EnqueueAudioObject 将音频多模态对象包装为 audio 信封并追加到队列。
// 接入阶段统一完成落盘、校验与转写，这里只做协议搬运。
func EnqueueAudioObject(object mediaobject.Object, source string) {
	refURL := strings.TrimSpace(object.URL)
	if refURL == "" && strings.TrimSpace(object.Data) != "" {
		format := strings.TrimSpace(object.Format)
		if format == "" {
			format = "wav"
		}
		refURL = "data:audio/" + format + ";base64," + object.Data
	}
	mediaSource := strings.TrimSpace(object.Source)
	if mediaSource == "" {
		mediaSource = mediaobject.SourceQQPrivate
	}
	EnqueueMessage(PostMessage{
		Role: RoleUser,
		Content: []ContentItem{{
			Type: ContentAudio,
			Audio: &mediaobject.AudioPayload{
				Ref:        mediaobject.Ref{Source: mediaSource, URL: refURL, MIME: object.MIME, Name: object.Name},
				DurationMS: object.DurationMS,
			},
		}},
		Source: source,
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
