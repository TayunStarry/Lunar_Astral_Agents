package napcat

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadBridgingConfigDirect 覆盖：专用 napcat_config.json 的根对象直接就是桥接配置。
func TestLoadBridgingConfigDirect(t *testing.T) {
	t.Cleanup(func() { bridgeConfig = BridgingConfig{} })
	path := filepath.Join(t.TempDir(), "napcat_config.json")
	data := `{"bridging_type":"napcat","bridging_path":"ws://127.0.0.1:4567","bridging_users":[10001,10002]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	if err := LoadBridgingConfig(path); err != nil {
		t.Fatalf("加载专用配置失败: %v", err)
	}
	if bridgeConfig.BridgingType != "napcat" || bridgeConfig.BridgingPath != "ws://127.0.0.1:4567" {
		t.Fatalf("专用配置未正确解析: %+v", bridgeConfig)
	}
	if len(bridgeConfig.BridgingUsers) != 2 {
		t.Fatalf("用户列表未正确解析: %+v", bridgeConfig.BridgingUsers)
	}
}

// TestLoadBridgingConfigLegacy 覆盖：旧版 lunar_config.json 的 bridging 组仍可读取。
func TestLoadBridgingConfigLegacy(t *testing.T) {
	t.Cleanup(func() { bridgeConfig = BridgingConfig{} })
	path := filepath.Join(t.TempDir(), "lunar_config.json")
	data := `{"bridging":{"bridging_type":"napcat","bridging_path":"ws://127.0.0.1:4568","bridging_users":[10003]}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	if err := LoadBridgingConfig(path); err != nil {
		t.Fatalf("加载旧版配置失败: %v", err)
	}
	if bridgeConfig.BridgingPath != "ws://127.0.0.1:4568" || len(bridgeConfig.BridgingUsers) != 1 {
		t.Fatalf("旧版配置未正确解析: %+v", bridgeConfig)
	}
}
