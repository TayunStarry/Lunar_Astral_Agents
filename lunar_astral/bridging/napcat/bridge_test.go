package napcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBridgingConfig(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "lunar_config.json")

	configData := map[string]interface{}{
		"server": map[string]interface{}{
			"bridging_type":     "napcat",
			"bridging_path":     "ws://localhost:4567",
			"bridging_token":    "test-token-123",
			"bridging_target":   262221051,
			"bridging_keywords": []string{"月华", "3826713076"},
		},
	}

	data, _ := json.Marshal(configData)
	os.WriteFile(configPath, data, 0644)

	err := LoadBridgingConfig(configPath)
	if err != nil {
		t.Fatalf("LoadBridgingConfig 失败: %v", err)
	}

	if bridgeConfig.BridgingType != "napcat" {
		t.Errorf("BridgingType = %q, 期望 'napcat'", bridgeConfig.BridgingType)
	}
	if bridgeConfig.BridgingPath != "ws://localhost:4567" {
		t.Errorf("BridgingPath = %q, 期望 'ws://localhost:4567'", bridgeConfig.BridgingPath)
	}
	if bridgeConfig.BridgingTarget != 262221051 {
		t.Errorf("BridgingTarget = %d, 期望 262221051", bridgeConfig.BridgingTarget)
	}
	if len(bridgeConfig.BridgingKeywords) != 2 {
		t.Errorf("BridgingKeywords 长度 = %d, 期望 2", len(bridgeConfig.BridgingKeywords))
	}
}

func TestLoadBridgingConfig_FileNotFound(t *testing.T) {
	err := LoadBridgingConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("文件不存在时应返回错误")
	}
}

func TestIsBridgingEnabled_Enabled(t *testing.T) {
	resetBridgeState()

	if !IsBridgingEnabled() {
		t.Error("bridging_type=napcat 且 bridging_path 非空时应启用")
	}
}

func TestIsBridgingEnabled_DisabledEmptyPath(t *testing.T) {
	resetBridgeState()
	bridgeConfig.BridgingPath = ""

	if IsBridgingEnabled() {
		t.Error("bridging_path 为空时不应启用")
	}
}

func TestIsBridgingEnabled_DisabledWrongType(t *testing.T) {
	resetBridgeState()
	bridgeConfig.BridgingType = "other"

	if IsBridgingEnabled() {
		t.Error("bridging_type 非 napcat 时不应启用")
	}
}

func TestGetBridgeState(t *testing.T) {
	resetBridgeState()

	if GetBridgeState() != BridgeDisconnected {
		t.Error("初始状态应为 BridgeDisconnected")
	}

	setBridgeState(BridgeConnected)
	if GetBridgeState() != BridgeConnected {
		t.Error("设置后状态应为 BridgeConnected")
	}
}

func TestHandleAgentResponse_MusicBlocked(t *testing.T) {
	resetBridgeState()
	setBridgeState(BridgeConnected)

	// 乐谱消息不应转发，不应panic
	HandleAgentResponse("music", "X:1\nT:Test")
}

func TestHandleAgentResponse_NotConnected(t *testing.T) {
	resetBridgeState()
	setBridgeState(BridgeDisconnected)

	// 未连接时不应转发
	HandleAgentResponse("text", "测试内容")
}

func TestStopBridge(t *testing.T) {
	resetBridgeState()

	StopBridge()

	if GetBridgeState() != BridgeDisconnected {
		t.Error("停止后状态应为 BridgeDisconnected")
	}
}
