package main

import (
	"net/http"
	"net/http/httputil"
)

// LoadApplicationRequest 加载应用请求结构体
type LoadApplicationRequest struct {
	Path string `json:"path"`
}

// LoadApplicationResponse 加载应用响应结构体
type LoadApplicationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// proxyAwareHandler 代理感知处理程序
// 用于在处理请求时根据路径判断是否需要通过代理转发
type proxyAwareHandler struct {
	fs          http.Handler
	proxy       *httputil.ReverseProxy
	shouldProxy func(string) bool
}
