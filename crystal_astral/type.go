package main

import (
	"net/http"
	"net/http/httputil"
)

// LoadApplicationRequest 加载应用请求结构体
type LoadApplicationRequest struct {
	Path string `json:"path"` // 应用路径
}

// LoadApplicationResponse 加载应用响应结构体
type LoadApplicationResponse struct {
	Success bool   `json:"success"`           // 是否成功加载应用
	Message string `json:"message,omitempty"` // 加载应用的消息提示
}

// PackageInfo 包配置信息
type PackageInfo struct {
	ID          string   `json:"id"`                     // 包ID，唯一标识一个应用
	Icon        string   `json:"icon,omitempty"`         // 包图标路径
	Title       string   `json:"title"`                  // 包标题，显示在应用列表中
	Description string   `json:"description"`            // 包描述，显示在应用列表中，描述应用的功能
	URL         string   `json:"url,omitempty"`          // 包的URL，用于下载应用
	Path        string   `json:"path,omitempty"`         // 包的本地路径，用于加载应用
	Tags        []string `json:"tags,omitempty"`         // 包的标签，用于分类应用
	PackageName string   `json:"package_name,omitempty"` // 包的名称，用于显示在应用列表中，描述应用的功能或来源
}

// proxyAwareHandler 代理感知处理程序
// 用于在处理请求时根据路径判断是否需要通过代理转发
type proxyAwareHandler struct {
	fs          http.Handler           // 文件系统处理程序，用于处理静态文件请求
	proxy       *httputil.ReverseProxy // 反向代理，用于将请求转发到其他服务器
	shouldProxy func(string) bool      // 判断是否需要通过代理转发的函数
}

// LunarCheckResponse 月华服务检测响应结构体
type LunarCheckResponse struct {
	Available bool `json:"available"` // 是否可用
}

// LunarStartResponse 月华服务启动响应结构体
type LunarStartResponse struct {
	Success bool   `json:"success"`           // 是否成功启动月华服务
	Message string `json:"message,omitempty"` // 启动月华服务的消息提示
}

// SystemEndpoint 系统端点
type SystemEndpoint struct {
	Path        string           // Path 端点路径
	Handler     http.HandlerFunc // Handler 处理函数
	Method      string           // Method 请求方法
	Description string           // Description 描述端点的功能
}

// ChatProxyRequest 对话代理请求结构体
type ChatProxyRequest struct {
	BaseURL  string                   `json:"base_url"`
	APIKey   string                   `json:"api_key"`
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// ChatProxyResponse 对话代理响应结构体
type ChatProxyResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ModelProxyRequest 模型代理请求结构体
type ModelProxyRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// ModelProxyResponse 模型代理响应结构体
type ModelProxyResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// GGUFMetadataRequest GGUF 元数据请求
type GGUFMetadataRequest struct {
	FilePath string `json:"filePath"`
}

// GGUFMetadataResponse GGUF 元数据响应
type GGUFMetadataResponse struct {
	Success  bool              `json:"success"`
	Error    string            `json:"error,omitempty"`
	FileName string            `json:"filename,omitempty"`
	FilePath string            `json:"filePath,omitempty"`
	Summary  map[string]string `json:"summary,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Count    int               `json:"count,omitempty"`
}

// ConvertImageRequest 图片转换请求
type ConvertImageRequest struct {
	Path         string `json:"path"`
	TargetFormat string `json:"target_format"`
	DeleteSource bool   `json:"delete_source"`
	Quality      int    `json:"quality"`
}

// ConvertImageResponse 图片转换响应
type ConvertImageResponse struct {
	Success    bool   `json:"success"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BatchConvertRequest 批量转换请求
type BatchConvertRequest struct {
	Folder       string `json:"folder"`
	SourceFormat string `json:"source_format"`
	TargetFormat string `json:"target_format"`
	DeleteSource bool   `json:"delete_source"`
	Quality      int    `json:"quality"`
}

// BatchConvertResult 批量转换单条结果
type BatchConvertResult struct {
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BatchConvertResponse 批量转换响应
type BatchConvertResponse struct {
	Success      bool                 `json:"success"`
	Results      []BatchConvertResult `json:"results"`
	Total        int                  `json:"total"`
	SuccessCount int                  `json:"success_count"`
	FailCount    int                  `json:"fail_count"`
	Error        string               `json:"error,omitempty"`
}

// ImageFileInfo 图片文件信息
type ImageFileInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Format string `json:"format"`
}

// ListImagesResponse 图片列表响应
type ListImagesResponse struct {
	Success bool            `json:"success"`
	Files   []ImageFileInfo `json:"files"`
	Folder  string          `json:"folder"`
	Error   string          `json:"error,omitempty"`
}
