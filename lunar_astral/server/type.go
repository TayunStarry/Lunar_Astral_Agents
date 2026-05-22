package server

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// SystemEndpoint 定义系统端点的结构
type SystemEndpoint struct {
	// HTTP 访问路径
	Path string `json:"path"`
	// HTTP 方法处理器
	Handler http.HandlerFunc `json:"handler"`
	// HTTP 方法类型
	Method string `json:"method"`
	// 处理器功能描述
	Description string `json:"description"`
}

// ModelConfig 定义模型配置的结构
type ModelConfig struct {
	// 模型配置
	Models struct {
		// 嵌入模型路径
		EmbeddingModel string `json:"embedding_model"`
		// 多模态模型路径
		MultimodalModel string `json:"multimodal_model"`
		// 多模态投影模型路径
		MmprojModel string `json:"mmproj_model"`
		// 扩散模型路径
		DiffusionModel string `json:"diffusion_model"`
		// 变分模型路径
		VariationalModel string `json:"variational_model"`
		// 提示精炼模型路径
		PromptRefineModel string `json:"prompt_refine_model"`
	} `json:"models"`
	// 服务器配置
	Server struct {
		// 是否为开发者模式
		Developer bool `json:"developer"`
		// 是否清除端口
		ClearPort bool `json:"clear_port"`
		// 是否允许加载扩散模型
		AllowDiffusion bool `json:"allow_diffusion"`
		// 是否允许加载多模态模型
		AllowMultimodal bool `json:"allow_multimodal"`
	} `json:"server"`
}

// WebSocket 客户端结构
type WSClient struct {
	// WebSocket 连接
	conn *websocket.Conn
	// 发送消息通道
	send chan []byte
	// 客户端引用
	client *WSClient
}

// WebSocket 消息结构
type WSMessage struct {
	// 消息类型
	Type string `json:"type"`
	// 消息数据
	Data any `json:"data,omitempty"`
}

// WebSocket 响应结构
type WSResponse struct {
	// 响应类型
	Type string `json:"type"`
	// 响应数据
	Data any `json:"data,omitempty"`
	// 响应上下文
	Context any `json:"context,omitempty"`
	// 响应图片
	Image any `json:"image,omitempty"`
}
