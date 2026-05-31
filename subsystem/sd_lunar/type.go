package main

import (
	"net/http"
	"net/http/httputil"
	"sync"
	"time"
)

// SystemEndpoint 系统端点
type SystemEndpoint struct {
	// Path 端点路径
	Path string
	// Handler 处理函数
	Handler http.HandlerFunc
	// Method 请求方法
	Method string
	// Description 描述端点的功能
	Description string
}

// GenerateTask 生成任务结构体
type GenerateTask struct {
	ID             string    `json:"id"`
	Prompt         string    `json:"prompt"`
	NegativePrompt string    `json:"negative_prompt"`
	BatchSize      int       `json:"batch_size"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	Strength       float64   `json:"strength"`
	Steps          int       `json:"steps"`
	Seed           int64     `json:"seed"`
	CfgScale       float64   `json:"cfg_scale"`
	InitImg        string    `json:"init_img"`
	UseVulkan      bool      `json:"use_vulkan"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
	ResultPath     string    `json:"result_path"`
	ResultBase64   string    `json:"result_base64"`
	Error          string    `json:"error"`
}

// proxyAwareHandler 代理感知处理程序
type proxyAwareHandler struct {
	fs          http.Handler
	proxy       *httputil.ReverseProxy
	shouldProxy func(string) bool
}

// Txt2ImgRequest 文生图请求结构体
type Txt2ImgRequest struct {
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`
	CfgScale       float64 `json:"cfg_scale"`
	Seed           int64   `json:"seed"`
	BatchSize      int     `json:"batch_size"`
	UseVulkan      bool    `json:"use_vulkan"`
	DiffusionModel string  `json:"diffusion_model"`
	VAEModel       string  `json:"vae_model"`
	RefineModel    string  `json:"refine_model"`
}

// Img2ImgRequest 图生图请求结构体
type Img2ImgRequest struct {
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`
	CfgScale       float64 `json:"cfg_scale"`
	Seed           int64   `json:"seed"`
	BatchSize      int     `json:"batch_size"`
	Strength       float64 `json:"strength"`
	InitImgBase64  string  `json:"init_img_base64"`
	UseVulkan      bool    `json:"use_vulkan"`
	DiffusionModel string  `json:"diffusion_model"`
	VAEModel       string  `json:"vae_model"`
	RefineModel    string  `json:"refine_model"`
}

// GenerateResponse 生成响应结构体
type GenerateResponse struct {
	Success  bool        `json:"success"`
	TaskID   string      `json:"task_id,omitempty"`
	Message  string      `json:"message,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Error    string      `json:"error,omitempty"`
}

var (
	TaskQueue     = make(chan GenerateTask, 10)
	TaskStatus    = make(map[string]*GenerateTask)
	TaskStatusMu  sync.RWMutex
	WaitClients   = make(map[string]chan *GenerateTask)
	WaitClientsMu sync.RWMutex
)