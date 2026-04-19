package processor

import (
	"strings"
	"subsystem/component/setup"
	"subsystem/component/utils"
)

// NewHandle 创建消息处理器
func NewHandle(config *setup.Config, wsClient *utils.Client) *Handle {
	return &Handle{
		Config:         config,
		wsClient:       wsClient,
		groupInfos:     make([]setup.GroupInfo, 0),
		groupMembers:   make(map[int64]map[int64]string),
		BaseURL:        strings.TrimSuffix(config.OpenAIAPIUrl, "/v1/chat/completions"),
		currentGroupID: 0,
	}
}
