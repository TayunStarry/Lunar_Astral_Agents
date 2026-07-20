package napcat

// 桥接器生命周期管理：配置加载、定时扫描、连接管理

import (
	"encoding/json"
	"os"
	"time"

	"logger"
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

// StartBridgeScanner 启动定时扫描机制
// 当未连接适配器且 bridging_path 不为空时，启动定时扫描
func StartBridgeScanner() {
	if !IsBridgingEnabled() {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未启用 (bridging_path 为空或 bridging_type 非 napcat)")
		return
	}

	logger.SubInfo("LunarCore", "Napcat", "桥接器配置: path=%s, target=%d, keywords=%v",
		bridgeConfig.BridgingPath, bridgeConfig.BridgingTarget, bridgeConfig.BridgingKeywords)

	// 启动首次扫描
	scheduleScan()
}

// scheduleScan 安排下一次扫描
func scheduleScan() {
	scanMutex.Lock()
	defer scanMutex.Unlock()

	if scanTimer != nil {
		scanTimer.Stop()
	}

	scanTimer = time.AfterFunc(scanInterval, performScan)
}

// performScan 执行一次扫描
func performScan() {
	scanMutex.Lock()
	currentRetry := scanRetryCount
	scanMutex.Unlock()

	if GetBridgeState() == BridgeConnected {
		return
	}

	logger.SubInfo("LunarCore", "Napcat", "正在扫描适配器连接 (第 %d/%d 次)...", currentRetry+1, maxScanRetries)

	// 尝试连接
	setBridgeState(BridgeConnecting)
	err := tryConnect()
	if err != nil {
		scanMutex.Lock()
		scanRetryCount++
		retryCount := scanRetryCount
		scanMutex.Unlock()

		if retryCount >= maxScanRetries {
			setBridgeState(BridgeFailed)
			logger.SubError("LunarCore", "Napcat", "达到最大重试次数 (%d)，停止扫描: %v", maxScanRetries, err)
			return
		}

		logger.SubWarn("LunarCore", "Napcat", "适配器连接失败，%v 后重试 (%d/%d): %v", scanInterval, retryCount, maxScanRetries, err)
		setBridgeState(BridgeDisconnected)
		scheduleScan()
		return
	}

	// 连接成功
	setBridgeState(BridgeConnected)
	scanMutex.Lock()
	scanRetryCount = 0
	scanMutex.Unlock()

	logger.SubInfo("LunarCore", "Napcat", "适配器连接成功")
}

// tryConnect 尝试连接适配器并启动消息处理循环
func tryConnect() error {
	err := ConnectToNapcatWebSocket(HandleNapcatMessage)
	if err != nil {
		setBridgeState(BridgeDisconnected)
		// 连接断开后，重新安排扫描（不消耗重试次数，仅连接失败消耗）
		go func() {
			if GetBridgeState() != BridgeFailed {
				logger.SubWarn("LunarCore", "Napcat", "连接断开，%v 后重新扫描...", scanInterval)
				scanMutex.Lock()
				scanRetryCount = 0
				scanMutex.Unlock()
				scheduleScan()
			}
		}()
		return err
	}
	return nil
}

// StopBridge 停止桥接器
func StopBridge() {
	scanMutex.Lock()
	if scanTimer != nil {
		scanTimer.Stop()
		scanTimer = nil
	}
	scanMutex.Unlock()

	setBridgeState(BridgeDisconnected)
	logger.SubInfo("LunarCore", "Napcat", "桥接器已停止")
}

// HandleAgentResponse 处理智能体的响应消息，转发回QQ群聊
// 严格保持乐谱消息不转发策略
func HandleAgentResponse(msgType string, content string) {
	// 乐谱消息不转发
	if msgType == "music" {
		logger.SubInfo("LunarCore", "Napcat", "乐谱消息已拦截，跳过转发")
		return
	}

	if GetBridgeState() != BridgeConnected {
		logger.SubInfo("LunarCore", "Napcat", "桥接器未连接，跳过消息转发")
		return
	}

	groupID := bridgeConfig.BridgingTarget
	if groupID == 0 {
		logger.SubError("LunarCore", "Napcat", "目标群号为空，无法转发消息")
		return
	}

	if err := SendGroupTextMessage(groupID, content); err != nil {
		logger.SubError("LunarCore", "Napcat", "转发消息到群 %d 失败: %v", groupID, err)
	}
}
