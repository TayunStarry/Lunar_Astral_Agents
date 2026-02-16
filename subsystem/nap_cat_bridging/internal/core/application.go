package core

import (
	"nap_cat_bridging/internal/config"    // 配置文件
	"nap_cat_bridging/internal/history"   // 消息历史管理器
	"nap_cat_bridging/internal/message"   // 消息处理器
	"nap_cat_bridging/internal/openai"    // OpenAI 客户端
	"nap_cat_bridging/internal/websocket" // WebSocket 客户端
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
	// 消息历史管理器
	HistoryManager *history.Manager
}

// NewApplication 创建应用程序实例
func NewApplication() (*Application, error) {
	// 加载配置
	cfg, err := config.Load("config.json")
	if err != nil {
		return nil, err
	}

	// 初始化WebSocket客户端
	wsClient := websocket.NewClient(cfg.NapCatWSServer, cfg.NapCatWSToken)

	// 初始化消息处理器
	messageHandler := message.NewProcessor(cfg, wsClient)

	// 初始化OpenAI客户端
	openAIClient := openai.NewClient(cfg)

	// 初始化消息历史管理器
	historyManager := history.NewManager(cfg)

	return &Application{
		Config:           cfg,
		WSClient:         wsClient,
		MessageProcessor: messageHandler,
		OpenAIClient:     openAIClient,
		HistoryManager:   historyManager,
	}, nil
}

// InitProcess 执行初始化流程
func (app *Application) InitProcess() error {
	// 连接到WebSocket服务器
	if err := app.WSClient.Connect(); err != nil {
		return err
	}

	// 发送获取群列表的请求
	_, err := app.WSClient.SendMessage("get_group_list", map[string]any{
		"no_cache": false,
	})
	if err != nil {
		return err
	}

	return nil
}
