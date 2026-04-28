package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

const MaxMessageCache = 5 // 最大缓存消息数量

var (
	AppConfig    Config
	GroupMembers = make(map[int64]map[int64]string)
	LastGroupID  int64           // 记录最新发送消息的群聊 ID
	MessageCache []CachedMessage // 消息缓存
)

// CachedMessage 缓存的消息结构
type CachedMessage struct {
	GroupID   int64
	UserID    int64
	Content   interface{} // 可以是 string 或 []map[string]interface{}
	HasImages bool
}

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

// AddToMessageCache 添加消息到缓存
func AddToMessageCache(msg CachedMessage) {
	MessageCache = append(MessageCache, msg)
	// 如果缓存超过最大数量，移除最旧的消息
	if len(MessageCache) > MaxMessageCache {
		MessageCache = MessageCache[1:]
	}
}

// ClearMessageCache 清除消息缓存
func ClearMessageCache() {
	MessageCache = nil
}

// HasTriggerKeywordInCache 检查缓存中是否有包含触发关键词的消息
func HasTriggerKeywordInCache() bool {
	for _, msg := range MessageCache {
		if str, ok := msg.Content.(string); ok {
			if ContainsTriggerKeyword(str) {
				return true
			}
		}
	}
	return false
}
