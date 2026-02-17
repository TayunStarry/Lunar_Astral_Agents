package core

import (
	"transponder/internal/config"    // 配置文件
	"transponder/internal/message"   // 消息处理器
	"transponder/internal/openai"    // OpenAI 客户端
	"transponder/internal/websocket" // WebSocket 客户端
)

// Application 应用程序结构
type Application struct {
	// 配置信息
	Config *config.Config
	// WebSocket 客户端
	WSClient *websocket.Client
	// 消息处理器
	MessageProcessor *message.Processor
	// OpenAI 客户端
	OpenAIClient *openai.Client
}

// NewApplication 创建应用程序实例
func NewApplication() (*Application, error) {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	// 初始化WebSocket客户端
	wsClient := websocket.NewClient(cfg.NapCatWSServer, cfg.NapCatWSToken)

	// 初始化消息处理器
	messageHandler := message.NewProcessor(cfg, wsClient)

	// 初始化OpenAI客户端
	openAIClient := openai.NewClient(cfg)

	return &Application{
		Config:           cfg,
		WSClient:         wsClient,
		MessageProcessor: messageHandler,
		OpenAIClient:     openAIClient,
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

	return nil
}
