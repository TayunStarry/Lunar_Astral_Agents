package setup

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

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
			DefaultReply:    "月华不知道哦~",
		}
		// 保存默认配置文件
		if err := saveConfig(DefaultConfigFile, defaultConfig); err != nil {
			return nil, fmt.Errorf("保存默认配置失败: %v", err)
		}
		// 显示创建默认配置文件信息
		log.Printf("已创建默认配置文件 %s", DefaultConfigFile)
		// 导出默认配置
		return defaultConfig, nil
	}
	// 读取配置文件
	configJSON, err := os.ReadFile(DefaultConfigFile)
	// 检查读取配置文件是否成功
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}
	// 定义配置结构体
	var config Config
	// 解析配置文件
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}
	// 显示读取配置文件信息
	log.Printf("已读取配置文件 %s", DefaultConfigFile)
	// 导出配置信息
	return &config, nil
}

// saveConfig 保存配置到文件
func saveConfig(configFile string, config *Config) error {
	// 序列化配置为JSON
	configJSON, err := json.MarshalIndent(config, "", "  ")
	// 检查序列化配置是否成功
	if err != nil {
		return err
	}
	// 执行写入配置文件操作
	if err := os.WriteFile(configFile, configJSON, 0644); err != nil {
		return err
	}
	// 返回成功
	return nil
}
