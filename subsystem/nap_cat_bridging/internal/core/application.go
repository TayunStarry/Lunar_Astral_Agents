package core

import (
	"nap_cat_bridging/internal/config"
	"nap_cat_bridging/internal/services/history"
	"nap_cat_bridging/internal/services/message"
	"nap_cat_bridging/pkg/openai"
	"nap_cat_bridging/pkg/websocket"
)

// Application 应用程序结构
type Application struct {
	Config         *config.Config
	WSClient       *websocket.Client
	MessageHandler *message.Handler
	OpenAIClient   *openai.Client
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
	messageHandler := message.NewHandler(cfg, wsClient)

	// 初始化OpenAI客户端
	openAIClient := openai.NewClient(cfg)

	// 初始化消息历史管理器
	historyManager := history.NewManager(cfg)

	return &Application{
		Config:         cfg,
		WSClient:       wsClient,
		MessageHandler: messageHandler,
		OpenAIClient:   openAIClient,
		HistoryManager: historyManager,
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
