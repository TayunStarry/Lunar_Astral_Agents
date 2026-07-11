package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/types"
)

const MaxMessageCache = 20

// AI 路由默认配置
const (
	DefaultAIRoutingModel = "system-multimodal"
	DefaultAIAPITimeout   = 30 // 秒
)

var (
	AppConfig    types.Config
	GroupMembers = make(map[int64]map[int64]string)
	LastGroupID  int64
	MessageCache []types.CachedMessage
)

func LoadConfig() {
	configFile := "local_data/lunar_config.json"

	configData, err := os.ReadFile(configFile)
	if err != nil {
		logger.Fatal("读取配置文件失败: %v", err)
	}

	if err := json.Unmarshal(configData, &AppConfig); err != nil {
		logger.Fatal("解析配置文件失败: %v", err)
	}

	logger.Info("成功读取配置文件")
	logger.Info("napcat_ws_server: %s", AppConfig.QQAdapter.NapcatWsServer)
	logger.Info("lunar_ws_server: %s", AppConfig.QQAdapter.LunarWsServer)
	logger.Info("listen_group_ids: %v", AppConfig.QQAdapter.ListenGroupIds)
	logger.Info("trigger_keywords: %v", AppConfig.QQAdapter.TriggerKeywords)
	logger.Info("ai_routing_enabled: %v, ai_routing_model: %s", AppConfig.QQAdapter.AIRoutingEnabled, GetAIRoutingModel())
}

func GetNapcatHTTPBaseURL() string {
	return strings.Replace(AppConfig.QQAdapter.NapcatWsServer, "ws://", "http://", 1)
}

func IsInListenGroup(groupID int64) bool {
	for _, id := range AppConfig.QQAdapter.ListenGroupIds {
		if id == groupID {
			return true
		}
	}
	return false
}

func GetRandomGroupID() int64 {
	if len(AppConfig.QQAdapter.ListenGroupIds) == 0 {
		return 0
	}
	return AppConfig.QQAdapter.ListenGroupIds[0]
}

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

func AddToMessageCache(msg types.CachedMessage) {
	MessageCache = append(MessageCache, msg)
	if len(MessageCache) > MaxMessageCache {
		MessageCache = MessageCache[1:]
	}
}

func ClearMessageCache() {
	MessageCache = nil
}

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

// GetAIRoutingEnabled 获取AI路由是否启用
func GetAIRoutingEnabled() bool {
	return AppConfig.QQAdapter.AIRoutingEnabled
}

// GetAIRoutingModel 获取AI路由使用的模型名称
func GetAIRoutingModel() string {
	if AppConfig.QQAdapter.AIRoutingModel == "" {
		return DefaultAIRoutingModel
	}
	return AppConfig.QQAdapter.AIRoutingModel
}

// GetLunarCoreV1URL 获取 lunar_core 的 /v1 端点地址
func GetLunarCoreV1URL() string {
	return AppConfig.QQAdapter.LunarCoreUrl + "/v1/chat/completions"
}

func GetUserName(groupID int64, userID int64) string {
	nickname := ""
	if groupID > 0 && userID > 0 {
		if members, ok := GroupMembers[groupID]; ok {
			if name, ok := members[userID]; ok {
				nickname = name
			}
		}
	}
	if nickname == "" {
		nickname = fmt.Sprintf("%d", userID)
	}
	return nickname
}
