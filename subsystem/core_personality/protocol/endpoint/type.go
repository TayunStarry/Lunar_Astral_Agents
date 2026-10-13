package endpoint

import "net/http"

// SystemEndpoint 单条系统端点定义（统一路由注册表的元素）。
// Handler 与 Path 由 server 装配层消费，Method/Description 供 /api-docs 与自检使用。
type SystemEndpoint struct {
	// Path 端点路径（ServeMux 模式，末尾带 / 表示子树匹配）
	Path string `json:"path"`
	// Handler 处理函数
	Handler http.HandlerFunc `json:"handler"`
	// Method 允许的方法（"GET"/"POST"/"ANY" 等，仅文档与自检用途）
	Method string `json:"method"`
	// Description 端点说明（供自述文档展示）
	Description string `json:"description"`
}
