package core

import (
	"transponder/internal/agent"     // Agent 客户端
	"transponder/internal/setup"    // 配置文件
	"transponder/internal/processor" // 核心处理器
	"transponder/internal/utils"     // 工具包
)

// Application 应用程序结构
type Application struct {
	// 配置信息
	Config *setup.Config
	// WebSocket 客户端
	WSClient *utils.Client
	// 消息处理器
	Processor *processor.Handle
	// OpenAI 客户端
	AgentClient *agent.Client
}
