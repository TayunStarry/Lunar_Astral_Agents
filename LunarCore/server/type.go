package server

import (
	"LunarCore/config"
	"LunarCore/model"
	"fmt"
	"net/http"
	"sync"
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

// httpMux 是HTTP服务器的ServeMux实例
var httpMux *http.ServeMux

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
		// TTS 服务地址
		TTSUrl string `json:"tts_url"`
		// 是否允许加载扩散模型
		AllowDiffusion bool `json:"allow_diffusion"`
		// 是否允许加载多模态模型
		AllowMultimodal bool `json:"allow_multimodal"`
	} `json:"server"`
}

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{fmt.Sprintf("http://localhost:%d", *config.BasicPort)}

// 请求映射，键为请求ID，值为请求上下文
var requests = make(map[string]*model.RequestContext)

// 互斥锁，用于保护请求映射的并发访问
var serverMutex sync.RWMutex
