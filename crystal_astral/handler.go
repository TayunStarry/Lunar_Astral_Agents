package main

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// getProxyHandler 获取代理处理程序
func getProxyHandler() *httputil.ReverseProxy {
	proxyURL, err := url.Parse("http://localhost:36789")
	if err != nil {
		LoggerGeneral.Error("CrystalAstral", "解析代理 URL 失败: %v", err)
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

	LoggerGeneral.Info("CrystalAstral", "Application started: %s", req.Path)

	go func() {
		if err := cmd.Wait(); err != nil {
			LoggerGeneral.Error("CrystalAstral", "Application %s exited with error: %v", req.Path, err)
		} else {
			LoggerGeneral.Info("CrystalAstral", "Application %s exited successfully", req.Path)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoadApplicationResponse{
		Success: true,
		Message: fmt.Sprintf("Application started: %s", req.Path),
	})
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

// scanPackagesHandler 扫描包目录，自动发现所有包含 metadata.json 的子文件夹
func scanPackagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get executable path: %v", err),
		})
		return
	}
	execDir := filepath.Dir(execPath)
	packageDir := filepath.Join(execDir, *GeneralConfig.LocalDir, "package")

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to read package directory: %v", err),
		})
		return
	}

	var packages []PackageInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(packageDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			// 没有 metadata.json 的子文件夹跳过（库/资源文件夹）
			continue
		}

		var pkg PackageInfo
		if err := json.Unmarshal(data, &pkg); err != nil {
			LoggerGeneral.Warn("CrystalAstral", "解析包配置失败 %s: %v", configPath, err)
			continue
		}

		// 设置包目录名
		pkg.PackageName = entry.Name()

		// 如果图标是相对路径，解析为完整 URL
		if pkg.Icon != "" && !strings.HasPrefix(pkg.Icon, "http") && !strings.HasPrefix(pkg.Icon, "/") {
			pkg.Icon = "/file/read/package/" + entry.Name() + "/" + pkg.Icon
		}

		// 如果未指定 url，自动生成默认路径
		if pkg.URL == "" && pkg.Path == "" {
			pkg.URL = "/file/read/package/" + entry.Name() + "/index.html"
		}

		packages = append(packages, pkg)
	}

	// 按标题排序，保证输出稳定
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ID < packages[j].ID
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(packages)
}

// yuehuaCheckHandler 检测月华服务端口(36789)是否可用
func yuehuaCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *GeneralConfig.BasicPort)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	available := err == nil
	if conn != nil {
		conn.Close()
	}

	writeJSON(w, http.StatusOK, LunarCheckResponse{
		Available: available,
	})
}

// yuehuaStartHandler 启动月华服务(Lunar_Astral.exe)
func yuehuaStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LunarStartResponse{
			Success: false,
			Message: fmt.Sprintf("获取可执行文件路径失败: %v", err),
		})
		return
	}
	execDir := filepath.Dir(execPath)
	exePath := filepath.Join(execDir, "Lunar_Astral.exe")

	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, LunarStartResponse{
			Success: false,
			Message: fmt.Sprintf("月华程序不存在: %s", exePath),
		})
		return
	}

	cmd := exec.Command(exePath)
	cmd.Dir = execDir
	// 为子进程分配独立控制台窗口，使其终端输出可见
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x10, // CREATE_NEW_CONSOLE
	}
	if err := cmd.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, LunarStartResponse{
			Success: false,
			Message: fmt.Sprintf("启动月华失败: %v", err),
		})
		return
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			LoggerGeneral.Error("CrystalAstral", "月华服务退出: %v", err)
		} else {
			LoggerGeneral.Info("CrystalAstral", "月华服务已退出")
		}
	}()

	LoggerGeneral.Info("CrystalAstral", "月华服务已启动: %s", exePath)
	writeJSON(w, http.StatusOK, LunarStartResponse{
		Success: true,
		Message: "月华服务启动成功",
	})
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
