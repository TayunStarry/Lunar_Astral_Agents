package napcat

// 桥接器生命周期与 AI 回应转发

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"os"
	"time"
)

// LoadBridgingConfig 从配置文件加载桥接器配置。
//
// 支持两种文件形态：
//   - 专用 napcat_config.json：根对象直接就是 BridgingConfig（新默认）；
//   - 旧版 lunar_config.json：配置位于 {"bridging": {...}} 组（升级兼容）。
func LoadBridgingConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	// 专用文件：根上有 bridging_type
	if _, ok := root["bridging_type"]; ok {
		var direct BridgingConfig
		if err := json.Unmarshal(data, &direct); err != nil {
			return err
		}
		bridgeConfig = direct
		return nil
	}
	// 旧版：{"bridging": {...}}
	if raw, ok := root["bridging"]; ok {
		var legacy BridgingConfig
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return err
		}
		bridgeConfig = legacy
		return nil
	}
	return nil
}

// IsBridgingEnabled 检查桥接器是否启用
func IsBridgingEnabled() bool {
	return bridgeConfig.BridgingPath != "" && bridgeConfig.BridgingType == "napcat"
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
	var lastErr error
	for {
		// 连接并阻塞读取消息，断开时返回错误，进入重连流程
		lastErr = ConnectToNapcatWebSocket(HandleNapcatMessage)
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
			lastErr = ConnectToNapcatWebSocket(HandleNapcatMessage)
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

// HandleAgentResponse 处理智能体的文本回应，转发到对应会话目标
//
// auto=true 表示这是高耗时任务的快速应答：照常转发给 QQ 用户，但**不占用应答份额**
// （不置 responseStarted，也不消费群聊的首次 @ 回执），真正的最终回应随后单独生成。
func HandleAgentResponse(msgType string, content string, auto bool) {
	flowMutex.Lock()
	if !awaitingResponse {
		// 无活跃QQ请求（如前端对话），不转发
		flowMutex.Unlock()
		return
	}
	target := currentTarget
	if !auto {
		responseStarted = true
	}
	flowMutex.Unlock()

	// 乐谱消息不转发
	if msgType == "music" {
		logger.SubInfo("LunarCore", "Napcat", "乐谱消息已拦截，跳过转发")
		return
	}
	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过消息转发")
		return
	}
	dispatchText(target, content, auto)
}

// HandleAgentImageResponse 处理智能体的图片回应，转发到对应会话目标
func HandleAgentImageResponse(images []string) {
	flowMutex.Lock()
	if !awaitingResponse {
		flowMutex.Unlock()
		return
	}
	target := currentTarget
	responseStarted = true
	flowMutex.Unlock()

	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过图片转发")
		return
	}
	dispatchImage(target, images)
}

// dispatchText 将文本回应转发到目标会话
//
// auto=true 时是快速应答：不消费群聊的首次 @ 回执，把 @ 留给真正的最终回应。
func dispatchText(target BridgeTarget, content string, auto bool) {
	if target.IsGroup {
		// 被 @ 触发的回应在首次发言时自动 @ 回发起者；取出后立即清零，整个回合只附加一次
		replyTo := int64(0)
		quoteID := int64(0)
		if !auto {
			flowMutex.Lock()
			replyTo = currentTarget.ReplyToUserID
			currentTarget.ReplyToUserID = 0
			// 随机触发的引用候选同样只消费一次：先摘除再离线计算，避免阻塞收发锁
			candidates := currentTarget.QuoteCandidates
			currentTarget.QuoteCandidates = nil
			flowMutex.Unlock()
			quoteID = pickQuoteMessageID(content, candidates)
		}

		if err := SendGroupTextMessage(target.ID, content, replyTo, quoteID); err != nil {
			logger.SubError("LunarCore", "Napcat", "转发消息到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := SendPrivateTextMessage(target.ID, content); err != nil {
		logger.SubError("LunarCore", "Napcat", "转发消息给用户 %d 失败: %v", target.ID, err)
	}
}

// dispatchImage 将图片回应转发到目标会话
func dispatchImage(target BridgeTarget, images []string) {
	if target.IsGroup {
		if err := SendGroupImageMessage(target.ID, images); err != nil {
			logger.SubError("LunarCore", "Napcat", "转发图片到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := SendPrivateImageMessage(target.ID, images); err != nil {
		logger.SubError("LunarCore", "Napcat", "转发图片给用户 %d 失败: %v", target.ID, err)
	}
}
