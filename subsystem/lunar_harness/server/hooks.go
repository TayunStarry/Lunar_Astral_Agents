package server

// hooks.go 跨层注入能力的 HTTP 适配层。
//
// 路由表在包初始化时构建，此时装配层尚未注入实现，因此这里放的是「取当前注入值」的
// 包装函数：请求到达时再解引用，避免把 nil 处理器注册进 ServeMux。

import (
	"net/http"
)

// messageBatchHandler 消息写入队列处理器（未注入时明确失败，便于定位装配顺序问题）。
func messageBatchHandler(w http.ResponseWriter, r *http.Request) {
	if MessageBatchHandlerFunc == nil {
		http.Error(w, "消息写入能力未装配", http.StatusServiceUnavailable)
		return
	}
	MessageBatchHandlerFunc(w, r)
}
