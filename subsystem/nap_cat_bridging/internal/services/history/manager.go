package history

import (
	"nap_cat_bridging/internal/config"
	"nap_cat_bridging/pkg/openai"
)

// Manager 消息历史管理器
type Manager struct {
	config              *config.Config
	groupMessageHistory map[int64][]openai.Message
}

// NewManager 创建消息历史管理器
func NewManager(config *config.Config) *Manager {
	return &Manager{
		config:              config,
		groupMessageHistory: make(map[int64][]openai.Message),
	}
}

// AddMessage 添加消息到历史
func (m *Manager) AddMessage(groupID int64, message openai.Message) {
	// 初始化该群的消息历史
	if _, ok := m.groupMessageHistory[groupID]; !ok {
		m.groupMessageHistory[groupID] = make([]openai.Message, 0)
	}

	// 添加消息
	m.groupMessageHistory[groupID] = append(m.groupMessageHistory[groupID], message)

	// 限制消息历史长度
	m.limitHistoryLength(groupID)
}

// GetMessages 获取群消息历史
func (m *Manager) GetMessages(groupID int64) []openai.Message {
	if messages, ok := m.groupMessageHistory[groupID]; ok {
		return messages
	}
	return []openai.Message{}
}

// ClearHistory 清空群消息历史
func (m *Manager) ClearHistory(groupID int64) {
	delete(m.groupMessageHistory, groupID)
}

// limitHistoryLength 限制消息历史长度
func (m *Manager) limitHistoryLength(groupID int64) {
	if messages, ok := m.groupMessageHistory[groupID]; ok {
		if len(messages) > m.config.MaxHistoryCount {
			m.groupMessageHistory[groupID] = messages[len(messages)-m.config.MaxHistoryCount:]
		}
	}
}
