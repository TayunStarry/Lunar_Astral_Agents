package config

// 配置加载与辅助函数

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"bridge_adapter/pkg/logger"
)

// LoadConfig 加载配置文件
func LoadConfig() {
	configFile := "local_data/lunar_config.json"

	configData, err := os.ReadFile(configFile)
	if err != nil {
		logger.Fatal("读取配置文件失败: %v", err)
	}

	if err := json.Unmarshal(configData, &AppConfig); err != nil {
		logger.Fatal("解析配置文件失败: %v", err)
	}

	// 填充默认值
	if AppConfig.QQAdapter.MaxGroupCache <= 0 {
		AppConfig.QQAdapter.MaxGroupCache = DefaultMaxGroupCache
	}
	if AppConfig.QQAdapter.MaxGroupSummary <= 0 {
		AppConfig.QQAdapter.MaxGroupSummary = DefaultMaxGroupSummary
	}

	logger.Info("成功读取配置文件")
	logger.Info("napcat_ws_server: %s", AppConfig.QQAdapter.NapcatWsServer)
	logger.Info("lunar_ws_server: %s", AppConfig.QQAdapter.LunarWsServer)
	logger.Info("listen_group_ids: %v", AppConfig.QQAdapter.ListenGroupIds)
	logger.Info("trigger_keywords: %v", AppConfig.QQAdapter.TriggerKeywords)
	logger.Info("ai_routing_enabled: %v, ai_routing_model: %s", AppConfig.QQAdapter.AIRoutingEnabled, GetAIRoutingModel())
	logger.Info("max_group_cache: %d, max_group_summary: %d", AppConfig.QQAdapter.MaxGroupCache, AppConfig.QQAdapter.MaxGroupSummary)
}

// GetNapcatHTTPBaseURL 将 napcat ws 地址转换为 http 地址
func GetNapcatHTTPBaseURL() string {
	return strings.Replace(AppConfig.QQAdapter.NapcatWsServer, "ws://", "http://", 1)
}

// IsInListenGroup 检查群号是否在监听列表中
func IsInListenGroup(groupID int64) bool {
	for _, id := range AppConfig.QQAdapter.ListenGroupIds {
		if id == groupID {
			return true
		}
	}
	return false
}

// GetRandomGroupID 获取一个可用的群号（取监听列表第一个）
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

// FindTriggerKeyword 返回消息中匹配到的第一个关键词，未匹配返回空字符串
func FindTriggerKeyword(message string) string {
	for _, keyword := range AppConfig.QQAdapter.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return keyword
		}
	}
	return ""
}

// GetUserName 获取群内用户昵称
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

// GetMaxGroupCache 获取每群最大缓存消息数
func GetMaxGroupCache() int {
	return AppConfig.QQAdapter.MaxGroupCache
}

// GetMaxGroupSummary 获取每群最大摘要数
func GetMaxGroupSummary() int {
	return AppConfig.QQAdapter.MaxGroupSummary
}
