package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// getProxyHandler 获取代理处理程序
func getProxyHandler() *httputil.ReverseProxy {
	proxyURL, err := url.Parse("http://localhost:36789")
	if err != nil {
		fmt.Printf("解析代理 URL 失败: %v\n", err)
		return nil
	}
	return httputil.NewSingleHostReverseProxy(proxyURL)
}

// loadApplicationHandler 处理加载应用的 HTTP 请求
func loadApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoadApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(req.Path))
	var cmd *exec.Cmd

	execPath, err := os.Executable()
	if err != nil {
		http.Error(w, "Failed to get executable path", http.StatusInternalServerError)
		return
	}
	execDir := filepath.Dir(execPath)

	if strings.HasPrefix(req.Path, "/") {
		req.Path = filepath.Join(execDir, req.Path[1:])
	}

	switch ext {
	case ".exe":
		cmd = exec.Command(req.Path)
	case ".ps1":
		cmd = exec.Command("powershell", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", req.Path)
	case ".bat":
		cmd = exec.Command("cmd", "/c", "start", "", req.Path)
	default:
		http.Error(w, "Unsupported file type: "+ext, http.StatusBadRequest)
		return
	}

	if !filepath.IsAbs(req.Path) {
		absPath, err := filepath.Abs(req.Path)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(LoadApplicationResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to resolve absolute path: %v", err),
			})
			return
		}
		req.Path = absPath
	}

	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoadApplicationResponse{
			Success: false,
			Message: fmt.Sprintf("File not found: %s", req.Path),
		})
		return
	}

	cmd.Dir = filepath.Dir(req.Path)

	if err := cmd.Start(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoadApplicationResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to start application: %v", err),
		})
		return
	}

	fmt.Printf("Application started: %s\n", req.Path)

	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Printf("Application %s exited with error: %v\n", req.Path, err)
		} else {
			fmt.Printf("Application %s exited successfully\n", req.Path)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoadApplicationResponse{
		Success: true,
		Message: fmt.Sprintf("Application started: %s", req.Path),
	})
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

// chatProxyHandler 代理 OpenAI 格式的对话请求
func chatProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.BaseURL == "" || req.Model == "" || len(req.Messages) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   "base_url, model, and messages are required",
		})
		return
	}

	normalizedURL := strings.TrimRight(req.BaseURL, "/")
	chatURL := normalizedURL + "/chat/completions"

	requestBody := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to marshal request: %v", err),
		})
		return
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	proxyReq, err := http.NewRequest("POST", chatURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create request: %v", err),
		})
		return
	}

	if req.APIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(proxyReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read response: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(body)),
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ChatProxyResponse{
			Success: false,
			Error:   "Failed to parse API response",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ChatProxyResponse{
		Success: true,
		Data:    result,
	})
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

// modelsProxyHandler 代理查询 OpenAI 格式的模型列表
func modelsProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ModelProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.BaseURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   "base_url is required",
		})
		return
	}

	normalizedURL := strings.TrimRight(req.BaseURL, "/")
	modelsURL := normalizedURL + "/models"

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	proxyReq, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create request: %v", err),
		})
		return
	}

	if req.APIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(proxyReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read response: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(body)),
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ModelProxyResponse{
			Success: false,
			Error:   "Failed to parse API response",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ModelProxyResponse{
		Success: true,
		Data:    result,
	})
}

// normalizeProxyURL 规范化代理 URL（仅去除末尾斜杠）
func normalizeProxyURL(rawURL string) string {
	return strings.TrimRight(rawURL, "/")
}
