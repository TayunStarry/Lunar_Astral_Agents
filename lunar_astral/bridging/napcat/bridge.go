package napcat

// 桥接器生命周期与 AI 回应转发

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"os"
)

// LoadBridgingConfig 从配置文件加载桥接器配置
func LoadBridgingConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var raw struct {
		Server BridgingConfig `json:"server"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	bridgeConfig = raw.Server
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
// 只允许连接一次：成功则持续服务，失败或断开则直接放弃该机制，不再重试
func StartBridgeScanner() {
	if !IsBridgingEnabled() {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器未启用 (bridging_path 为空或 bridging_type 非 napcat)")
		return
	}

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器配置: path=%s, targets=%v",
		bridgeConfig.BridgingPath, bridgeConfig.BridgingUsers)

	// 单次连接尝试，成功后阻塞服务直至断开，失败后不再重试
	setBridgeState(BridgeConnecting)
	err := ConnectToNapcatWebSocket(HandleNapcatMessage)

	// 走到这里说明连接失败或连接已断开，直接放弃桥接机制
	setBridgeState(BridgeFailed)
	LoggerGeneral.SubError("LunarCore", "Napcat", "适配器连接失败或已断开，放弃桥接机制: %v", err)
}

// StopBridge 停止桥接器
func StopBridge() {
	setBridgeState(BridgeDisconnected)
	LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器已停止")
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
func HandleAgentResponse(msgType string, content string) {
	flowMutex.Lock()
	if !awaitingResponse {
		// 无活跃QQ请求（如前端对话），不转发
		flowMutex.Unlock()
		return
	}
	target := currentTarget
	responseStarted = true
	flowMutex.Unlock()

	// 乐谱消息不转发
	if msgType == "music" {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "乐谱消息已拦截，跳过转发")
		return
	}
	if GetBridgeState() != BridgeConnected {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过消息转发")
		return
	}
	dispatchText(target, content)
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
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过图片转发")
		return
	}
	dispatchImage(target, images)
}

// dispatchText 将文本回应转发到目标会话
func dispatchText(target BridgeTarget, content string) {
	if target.IsGroup {
		if err := SendGroupTextMessage(target.ID, content); err != nil {
			LoggerGeneral.SubError("LunarCore", "Napcat", "转发消息到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := SendPrivateTextMessage(target.ID, content); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "转发消息给用户 %d 失败: %v", target.ID, err)
	}
}

// dispatchImage 将图片回应转发到目标会话
func dispatchImage(target BridgeTarget, images []string) {
	if target.IsGroup {
		if err := SendGroupImageMessage(target.ID, images); err != nil {
			LoggerGeneral.SubError("LunarCore", "Napcat", "转发图片到群 %d 失败: %v", target.ID, err)
		}
		return
	}
	if err := SendPrivateImageMessage(target.ID, images); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "转发图片给用户 %d 失败: %v", target.ID, err)
	}
}
