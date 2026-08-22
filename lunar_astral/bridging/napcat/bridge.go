package napcat

// 桥接器生命周期管理：配置加载、定时扫描、连接管理

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

// GetBridgingKeywords 获取桥接器关键词列表（供入站消息关键词检测使用）
func GetBridgingKeywords() []string {
	return bridgeConfig.BridgingKeywords
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

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器配置: path=%s, target=%d, keywords=%v",
		bridgeConfig.BridgingPath, bridgeConfig.BridgingTarget, bridgeConfig.BridgingKeywords)

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

// HandleAgentResponse 处理智能体的文本响应消息，转发回QQ群聊
// 严格保持乐谱消息不转发策略
func HandleAgentResponse(msgType string, content string) {
	// 乐谱消息不转发
	if msgType == "music" {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "乐谱消息已拦截，跳过转发")
		return
	}

	if GetBridgeState() != BridgeConnected {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过消息转发")
		return
	}

	groupID := bridgeConfig.BridgingTarget
	if groupID == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "目标群号为空，无法转发消息")
		return
	}

	if err := SendGroupTextMessage(groupID, content); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "转发消息到群 %d 失败: %v", groupID, err)
	}
}

// HandleAgentImageResponse 处理智能体的图片响应消息，转发回QQ群聊
func HandleAgentImageResponse(images []string) {
	if GetBridgeState() != BridgeConnected {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过图片转发")
		return
	}

	groupID := bridgeConfig.BridgingTarget
	if groupID == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "目标群号为空，无法转发图片")
		return
	}

	if err := SendGroupImageMessage(groupID, images); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "转发图片到群 %d 失败: %v", groupID, err)
	}
}
