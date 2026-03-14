package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// ProxyResponse 代理响应结构

type ProxyResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}

// handleProxyRequest 处理代理请求
func handleProxyRequest(req ProxyRequest) ([]byte, string, error) {
	// 验证URL
	if req.URL == "" {
		return nil, "", fmt.Errorf("URL is required")
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 准备请求体
	var reqBody io.Reader
	if req.RequestInit.Body != nil {
		bodyBytes, err := json.Marshal(req.RequestInit.Body)
		if err != nil {
			log.Printf("序列化请求体失败: %v", err)
			return nil, "", fmt.Errorf("Invalid request body")
		}
		reqBody = bytes.NewBuffer(bodyBytes)
	}

	// 获取请求方法
	method := "GET"
	if req.RequestInit.Method != "" {
		method = req.RequestInit.Method
	}

	// 创建请求
	httpReq, err := http.NewRequest(method, req.URL, reqBody)
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return nil, "", fmt.Errorf("Failed to create request")
	}

	// 设置请求头
	if req.RequestInit.Headers != nil {
		for key, value := range req.RequestInit.Headers {
			httpReq.Header.Set(key, value)
		}
	}

	// 发送请求
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("发送请求失败: %v", err)
		return nil, "", fmt.Errorf("Failed to send request")
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应体失败: %v", err)
		return nil, "", fmt.Errorf("Failed to read response")
	}

	// 检查响应是否为图片
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "image/") {
		return respBody, contentType, nil
	}

	// 转换headers为map[string]string
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// 构建JSON响应
	response := ProxyResponse{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    headers,
		Body:       json.RawMessage(respBody),
	}

	// 编码响应
	jsonResp, err := json.Marshal(response)
	if err != nil {
		log.Printf("编码响应失败: %v", err)
		return nil, "", fmt.Errorf("Failed to encode response")
	}

	return jsonResp, "application/json", nil
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

	// 执行代理请求
	respData, contentType, err := handleProxyRequest(proxyReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头并返回数据
	w.Header().Set("Content-Type", contentType)
	w.Write(respData)
}
