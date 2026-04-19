package server

import (
	"LunarCore/config"
	"LunarCore/model"
	"fmt"
	"net/http"
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
	Models struct {
		// 嵌入模型路径
		Embedding string `json:"embedding"`
		// 多模态模型路径
		Multimodal string `json:"multimodal"`
		// 多模态投影模型路径
		MultimodalMmproj string `json:"multimodal_mmproj"`
		// 扩散模型路径
		Diffusion string `json:"diffusion"`
		// 变分模型路径
		Variational string `json:"variational"`
		// 提示精炼模型路径
		PromptRefine string `json:"prompt_refine"`
	} `json:"models"`
}

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{fmt.Sprintf("http://localhost:%d", *config.BasicPort)}

// 请求映射，键为请求ID，值为请求上下文
var requests = make(map[string]*model.RequestContext)

// 互斥锁，用于保护请求映射的并发访问
var serverMutex sync.RWMutex
