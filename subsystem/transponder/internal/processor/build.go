package processor

import (
	"strings"
	"transponder/internal/setup"
	"transponder/internal/utils"
)

// NewHandle 创建消息处理器
func NewHandle(config *setup.Config, wsClient *utils.Client) *Handle {
	return &Handle{
		Config:         config,
		wsClient:       wsClient,
		groupInfos:     make([]setup.GroupInfo, 0),
		groupMembers:   make(map[int64]map[int64]string),
		baseURL:        strings.TrimSuffix(config.OpenAIAPIUrl, "/v1/chat/completions"),
		currentGroupID: 0,
	}
}
