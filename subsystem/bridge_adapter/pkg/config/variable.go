package config

// 所有全局变量与常量声明

import "bridge_adapter/pkg/types"

// 默认值常量
const (
	DefaultAIRoutingModel  = "system-multimodal"
	DefaultAIAPITimeout    = 30 // 秒
	DefaultMaxGroupCache   = 20 // 每群最大缓存消息数
	DefaultMaxGroupSummary = 10 // 每群最大摘要数
)

var (
	AppConfig    types.Config
	GroupMembers = make(map[int64]map[int64]string) // groupID -> userID -> nickname
	LastGroupID  int64                              // 最近触发关键词的群号
)
