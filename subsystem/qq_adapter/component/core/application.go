package core

import (
	"subsystem/component/agent"     // OpenAI 客户端
	"subsystem/component/processor" // 核心处理器
	"subsystem/component/setup"     // 配置文件
	"subsystem/component/utils"     // 工具包
)

// NewApplication 创建应用程序实例
func NewApplication() (*Application, error) {
	// 加载配置
	cfg, err := setup.Load()
	// 检查加载配置是否成功
	if err != nil {
		return nil, err
	}
	// 初始化WebSocket客户端
	wsClient := utils.NewClient(cfg.NapCatWSServer, cfg.NapCatWSToken)
	// 初始化消息处理器
	messageHandler := processor.NewHandle(cfg, wsClient)
	// 初始化OpenAI客户端
	agentClient := agent.NewClient(cfg)
	// 初始化应用程序实例
	return &Application{
		Config:      cfg,
		WSClient:    wsClient,
		Processor:   messageHandler,
		AgentClient: agentClient,
	}, nil
}

// InitProcess 执行初始化流程
func (class *Application) InitProcess() error {
	// 连接到WebSocket服务器
	if err := class.WSClient.Connect(); err != nil {
		return err
	}
	// 发送获取群列表的请求
	_, err := class.WSClient.SendMessage("get_group_list", map[string]any{"no_cache": false})
	if err != nil {
		return err
	}
	// 返回成功
	return nil
}
