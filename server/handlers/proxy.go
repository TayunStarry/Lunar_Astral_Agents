package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ProxyRequest 定义代理请求的结构
type ProxyRequest struct {
	URL         string      `json:"url"`
	RequestInit RequestInit `json:"requestInit"`
}

// RequestInit 定义请求初始化参数的结构
type RequestInit struct {
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
	Redirect    string            `json:"redirect,omitempty"`
	Credentials string            `json:"credentials,omitempty"`
}

// ProxyHandler 处理代理请求
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 处理OPTIONS请求
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 检查请求方法
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var proxyReq ProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&proxyReq); err != nil {
		log.Printf("解析代理请求失败: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证URL
	if proxyReq.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 准备请求体
	var reqBody io.Reader
	if proxyReq.RequestInit.Body != nil {
		bodyBytes, err := json.Marshal(proxyReq.RequestInit.Body)
		if err != nil {
			log.Printf("序列化请求体失败: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		reqBody = bytes.NewBuffer(bodyBytes)
	}

	// 获取请求方法
	method := "GET"
	if proxyReq.RequestInit.Method != "" {
		method = proxyReq.RequestInit.Method
	}

	// 创建请求
	req, err := http.NewRequest(method, proxyReq.URL, reqBody)
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 设置请求头
	if proxyReq.RequestInit.Headers != nil {
		for key, value := range proxyReq.RequestInit.Headers {
			req.Header.Set(key, value)
		}
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("发送请求失败: %v", err)
		http.Error(w, "Failed to send request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应体失败: %v", err)
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	// 构建响应
	response := map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    resp.Header,
		"body":       json.RawMessage(respBody),
	}

	// 检查响应是否为图片
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "image/") {
		// 直接返回图片数据
		w.Header().Set("Content-Type", contentType)
		w.Write(respBody)
		return
	}

	// 发送JSON响应
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("编码响应失败: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
