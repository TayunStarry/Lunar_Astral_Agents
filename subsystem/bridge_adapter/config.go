package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

var (
	AppConfig    Config
	GroupMembers = make(map[int64]map[int64]string)
	LastGroupID  int64 // 记录最新发送消息的群聊 ID
)

// LoadConfig 加载配置文件
func LoadConfig() {
	configFile := "local_data/lunar_config.json"

	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	if err := json.Unmarshal(configData, &AppConfig); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	log.Println("成功读取配置文件")
	log.Printf("napcat_ws_server: %s", AppConfig.QQAdapter.NapcatWsServer)
	log.Printf("lunar_ws_server: %s", AppConfig.QQAdapter.LunarWsServer)
	log.Printf("listen_group_ids: %v", AppConfig.QQAdapter.ListenGroupIds)
	log.Printf("trigger_keywords: %v", AppConfig.QQAdapter.TriggerKeywords)
}

// GetNapcatHTTPBaseURL 获取 Napcat HTTP 基础 URL
func GetNapcatHTTPBaseURL() string {
	return strings.Replace(AppConfig.QQAdapter.NapcatWsServer, "ws://", "http://", 1)
}

// IsInListenGroup 检查群 ID 是否在监听列表中
func IsInListenGroup(groupID int64) bool {
	for _, id := range AppConfig.QQAdapter.ListenGroupIds {
		if id == groupID {
			return true
		}
	}
	return false
}

// GetRandomGroupID 获取随机的群 ID
func GetRandomGroupID() int64 {
	if len(AppConfig.QQAdapter.ListenGroupIds) == 0 {
		return 0
	}
	return AppConfig.QQAdapter.ListenGroupIds[0]
}

// ContainsTriggerKeyword 检查消息是否包含触发关键词
func ContainsTriggerKeyword(message string) bool {
	if len(AppConfig.QQAdapter.TriggerKeywords) == 0 {
		return true
	}
	for _, keyword := range AppConfig.QQAdapter.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
