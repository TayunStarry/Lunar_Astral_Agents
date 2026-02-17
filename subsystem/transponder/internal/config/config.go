package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// Config 配置结构体
type Config struct {
	// NapCat WebSocket 服务器地址
	NapCatWSServer string `json:"napcat_ws_server"`
	// NapCat WebSocket 服务器令牌
	NapCatWSToken string `json:"napcat_ws_token"`
	// OpenAI API URL
	OpenAIAPIUrl string `json:"openai_api_url"`
	// OpenAI API 令牌
	OpenAIAPIToken string `json:"openai_api_token"`
	// OpenAI API 模型
	OpenAIAPIModel string `json:"openai_api_model"`
	// 轮询间隔（秒）
	PollInterval int `json:"poll_interval"`
	// 监听的群 ID 列表
	ListenGroupIDs []int64 `json:"listen_group_ids"`
	// 触发关键词列表
	TriggerKeywords []string `json:"trigger_keywords"`
}

var (
	// DefaultConfigFile 默认配置文件路径
	DefaultConfigFile = "config.json"
	// DisplayDebugMessage 是否显示调试信息
	DisplayDebugMessage = false
)

// Load 加载配置文件
func Load() (*Config, error) {
	// 检查配置文件是否存在
	if _, err := os.Stat(DefaultConfigFile); os.IsNotExist(err) {
		// 创建默认配置文件
		defaultConfig := &Config{
			NapCatWSServer:  "ws://localhost:20485",
			NapCatWSToken:   "ItlC2Nc1DfICVYq5",
			OpenAIAPIUrl:    "http://localhost:36794/v1/chat/completions",
			OpenAIAPIToken:  "",
			OpenAIAPIModel:  "system-multimodal",
			PollInterval:    10,
			ListenGroupIDs:  []int64{11223344},
			TriggerKeywords: []string{"月之华", "月华"},
		}

		if err := saveConfig(DefaultConfigFile, defaultConfig); err != nil {
			return nil, fmt.Errorf("保存默认配置失败: %v", err)
		}

		log.Printf("已创建默认配置文件 %s", DefaultConfigFile)
		return defaultConfig, nil
	}

	// 读取配置文件
	configJSON, err := os.ReadFile(DefaultConfigFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	log.Printf("已读取配置文件 %s", DefaultConfigFile)
	return &config, nil
}

// saveConfig 保存配置到文件
func saveConfig(configFile string, config *Config) error {
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configFile, configJSON, 0644); err != nil {
		return err
	}

	return nil
}
