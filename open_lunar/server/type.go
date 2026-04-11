package server

import (
	"fmt"
	"net/http"
	"open-lunar/config"
	"open-lunar/file_system/model"
	"sync"
)

// IPInfo 存储IP地址信息
type IPInfo struct {
	Region string `json:"region"`
	City   string `json:"city"`
}

// 全局变量
var websocketServer *http.Server

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
	// 嵌入模型路径
	EmbeddingModelPath string `json:"embedding_model_path"`
	// 多模态模型路径
	MultimodalModelPath string `json:"multimodal_model_path"`
	// 多模态投影模型路径
	MmprojModelPath string `json:"mmproj_model_path"`
	// 扩散模型路径
	DiffusionModelPath string `json:"diffusion_model_path"`
	// 变分模型路径
	VariationalModelPath string `json:"variational_model_path"`
	// 提示精炼模型路径
	PromptRefineModelPath string `json:"prompt_refine_model_path"`
}

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{fmt.Sprintf("http://localhost:%d", *config.BasicPort)}

// 请求映射，键为请求ID，值为请求上下文
var requests = make(map[string]*model.RequestContext)

// 互斥锁，用于保护请求映射的并发访问
var serverMutex sync.RWMutex
