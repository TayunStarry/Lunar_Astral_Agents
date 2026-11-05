package DialogueBridging

// 桥接器生命周期与 AI 回应转发

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"os"
	"time"
)

// LoadBridgingConfig 从专用配置文件 config/napcat.json 加载桥接器配置。
func LoadBridgingConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var direct BridgingConfig
	if err := json.Unmarshal(data, &direct); err != nil {
		return err
	}
	bridgeConfig = direct
	return nil
}

// IsBridgingEnabled 检查桥接器是否启用
func IsBridgingEnabled() bool {
	return bridgeConfig.BridgingPath != "" && bridgeConfig.BridgingType == "napcat"
}

// BridgingEndpoint 返回桥接 WebSocket 地址与鉴权 token，供装配层构造传输客户端。
func BridgingEndpoint() (string, string) {
	return bridgeConfig.BridgingPath, bridgeConfig.BridgingToken
}

// GetBridgeState 获取桥接器当前状态
func GetBridgeState() BridgeState {
	bridgeStateMutex.RLock()
	defer bridgeStateMutex.RUnlock()
	return bridgeState
}

// setBridgeState 设置桥接器状态
func setBridgeState(state BridgeState) {
	bridgeStateMutex.Lock()
	defer bridgeStateMutex.Unlock()
	bridgeState = state
}

// StartBridgeScanner 启动桥接器连接
// 成功后持续服务；断联时每隔 5 秒重连 1 次，单次断联最多重试 3 次
func StartBridgeScanner() {
	if !IsBridgingEnabled() {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未启用 (bridging_path 为空或 bridging_type 非 napcat)")
		return
	}

	logger.SubInfo("LunarCore", "Napcat", "桥接器配置: path=%s, targets=%v",
		bridgeConfig.BridgingPath, bridgeConfig.BridgingUsers)

	// 维持连接并处理断线重连，重试耗尽后放弃桥接机制
	setBridgeState(BridgeConnecting)
	err := connectWithRetry()

	setBridgeState(BridgeFailed)
	logger.SubError("LunarCore", "Napcat", "适配器重试 %d 次后仍无法连接，放弃桥接机制: %v", bridgeMaxReconnectAttempts, err)
}

// connectWithRetry 维持桥接连接：断联时每隔 5 秒重连 1 次，单次断联最多重试 3 次
// 连接成功后重连机会重置，下一次断联重新获得 3 次重连机会
func connectWithRetry() error {
	onConnected := func() { setBridgeState(BridgeConnected) }
	onDisconnected := func() { setBridgeState(BridgeDisconnected) }
	var lastErr error
	for {
		// 连接并阻塞读取消息，断开时返回错误，进入重连流程
		lastErr = napcat.Connect(HandleNapcatMessage, onConnected, onDisconnected)
		if lastErr == nil {
			return nil
		}

		// 单次断联最多重试 3 次，全部失败则放弃桥接机制
		reconnected := false
		for attempt := 1; attempt <= bridgeMaxReconnectAttempts; attempt++ {
			logger.SubInfo("LunarCore", "Napcat", "连接已断开，%d 秒后进行第 %d/%d 次重连",
				bridgeReconnectInterval/time.Second, attempt, bridgeMaxReconnectAttempts)
			time.Sleep(bridgeReconnectInterval)
			setBridgeState(BridgeConnecting)
			lastErr = napcat.Connect(HandleNapcatMessage, onConnected, onDisconnected)
			if lastErr == nil {
				reconnected = true
				break
			}
		}
		if !reconnected {
			return lastErr
		}
	}
}

// StopBridge 停止桥接器
func StopBridge() {
	setBridgeState(BridgeDisconnected)
	logger.SubInfo("LunarCore", "Napcat", "桥接器已停止")
}

// NotifyAgentIdle 由智能体上下文拉取层调用：当智能体本轮无待处理消息（拉取为空）时通知桥接器。
// 若存在已推送且已收到回应的请求，则认为上一轮QQ对话已回应完，推进队列中的下一条请求。
func NotifyAgentIdle() {
	flowMutex.Lock()
	if awaitingResponse && responseStarted {
		awaitingResponse = false
		responseStarted = false
		flowMutex.Unlock()
		pumpNext()
		return
	}
	flowMutex.Unlock()
}

// takeDelivery 在 flowMutex 内快照当前投递目标，并按消息语义处理一次性份额。
//
// final=false：本轮尚未收尾的旁白与过程产物（快速应答、工具轮次正文、思考块，
// 以及工具轮次中产出的音乐与图片）。只快照目标——不置 responseStarted，也不消费
// 群聊的一次性 @ 回执与引用候选。否则空闲信号会在终稿落地前释放应答份额，队列里
// 的下一条请求顶替 currentTarget，终稿与这些产物都会投错会话。
//
// final=true：本轮的收尾消息（终稿文本）。置位 responseStarted，并把 @ 回执与引用
// 候选与 target 快照在同一临界区内摘除，保证它们恰好被终稿消费一次；若留到投递时
// 再读 currentTarget，空闲推进可能已换过目标。
//
// ok=false 表示当前没有 QQ 请求（前端对话等），调用方直接丢弃。
func takeDelivery(final bool) (target BridgeTarget, replyTo int64, candidates []QuoteCandidate, ok bool) {
	flowMutex.Lock()
	defer flowMutex.Unlock()
	if !awaitingResponse {
		return BridgeTarget{}, 0, nil, false
	}
	target = currentTarget
	if !final {
		return target, 0, nil, true
	}
	responseStarted = true
	replyTo = currentTarget.ReplyToUserID
	currentTarget.ReplyToUserID = 0
	candidates = currentTarget.QuoteCandidates
	currentTarget.QuoteCandidates = nil
	return target, replyTo, candidates, true
}

// HandleAgentResponse 处理智能体的文本回应，转发到对应会话目标
//
// auto=true 表示这是本轮尚未收尾的消息（快速应答或工具轮次正文）：照常转发给 QQ
// 用户，但不占用应答份额，真正的最终回应随后单独生成。
func HandleAgentResponse(msgType string, content string, auto bool) {
	// 音乐消息由结构化旁路单独转发（见 HandleAgentMusicResponse），这里不再处理
	if msgType == "music" {
		return
	}
	target, replyTo, candidates, ok := takeDelivery(!auto)
	if !ok {
		// 无活跃QQ请求（如前端对话），不转发
		return
	}
	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过消息转发")
		return
	}
	quoteID := int64(0)
	if len(candidates) > 0 {
		// 相关性计算放到锁外，避免阻塞收发
		quoteID = pickQuoteMessageID(content, candidates)
	}
	dispatchText(target, content, replyTo, quoteID)
}

// HandleAgentImageResponse 处理智能体的图片回应，转发到对应会话目标。
//
// 图片在工具轮次中途产出（表情包、截图、绘制结果），本轮真正的收尾消息是随后
// 推送的终稿文本：按旁白语义只快照目标，不置 responseStarted、不消费 @ / 引用。
func HandleAgentImageResponse(images []string) {
	target, _, _, ok := takeDelivery(false)
	if !ok {
		return
	}
	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过图片转发")
		return
	}
	dispatchImage(target, images)
}

// HandleAgentMusicResponse 处理智能体的音乐回应，转发到对应会话目标。
// 与文本/图片一致：音乐在工具轮次中途产出，终稿文本随后收尾，因此同样按旁白
// 语义只快照目标，不置 responseStarted、不消费 @ / 引用。前端收到的仍是同一条
// 结构化广播，不在这里重新生成或重新渲染。
func HandleAgentMusicResponse(audio, title, summary string, duration float64) {
	target, _, _, ok := takeDelivery(false)
	if !ok {
		// Web 端会话或没有待回应的 QQ 请求，不转发
		return
	}
	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过音乐转发")
		return
	}
	dispatchMusic(target, audio, title, summary, duration)
}

// dispatchMusic 将演奏音频转发到目标会话：优先语音条，失败降级为文字 + 音频文件。
func dispatchMusic(target BridgeTarget, audio, title, summary string, duration float64) {
	if err := napcat.SendMusic(target.IsGroup, target.ID, audio, title, summary, duration); err != nil {
		where := "群"
		if !target.IsGroup {
			where = "用户"
		}
		logger.SubError("LunarCore", "Napcat", "转发音乐到%s %d 失败: %v", where, target.ID, err)
	}
}

// dispatchText 将文本回应转发到目标会话
//
// replyTo / quoteID 已由 HandleAgentResponse 在与 target 快照相同的临界区内一次性取出，
// 这里只做投递，不再读写流程状态，避免空闲推进后误消费下一条请求的 @ / 引用。
func dispatchText(target BridgeTarget, content string, replyTo int64, quoteID int64) {
	if target.IsGroup {
		if err := napcat.SendGroupTextMessage(target.ID, content, replyTo, quoteID); err != nil {
			logger.SubError("LunarCore", "Napcat", "转发消息到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := napcat.SendPrivateTextMessage(target.ID, content); err != nil {
		logger.SubError("LunarCore", "Napcat", "转发消息给用户 %d 失败: %v", target.ID, err)
	}
}

// dispatchImage 将图片回应转发到目标会话
func dispatchImage(target BridgeTarget, images []string) {
	if target.IsGroup {
		if err := napcat.SendGroupImageMessage(target.ID, images); err != nil {
			logger.SubError("LunarCore", "Napcat", "转发图片到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := napcat.SendPrivateImageMessage(target.ID, images); err != nil {
		logger.SubError("LunarCore", "Napcat", "转发图片给用户 %d 失败: %v", target.ID, err)
	}
}
