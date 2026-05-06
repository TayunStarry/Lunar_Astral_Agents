package main

import (
	"config"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// getRandomBackgroundImage 从 background 目录中随机选择一个背景图片文件名
func getRandomBackgroundImage() (string, error) {
	backgroundDir := filepath.Join(*config.LocalDir, "images/background")
	entries, err := os.ReadDir(backgroundDir)
	if err != nil {
		return "", err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "picture") {
			files = append(files, entry.Name())
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("未找到 picture 开头的图片文件")
	}
	randomIndex := rand.Intn(len(files))
	return files[randomIndex], nil
}

// RandomBackgroundHandler 服务随机选择的背景图片
func RandomBackgroundHandler(w http.ResponseWriter, _ *http.Request) {
	filename, err := getRandomBackgroundImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	backgroundDir := filepath.Join(*config.LocalDir, "images/background")
	filePath := filepath.Join(backgroundDir, filename)
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "无法打开文件: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(filename))
	contentType := ""
	switch ext {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	default:
		contentType = "application/octet-stream"
	}

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "无法获取文件信息: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.WriteHeader(http.StatusOK)

	if _, err := copyBuffer(w, file); err != nil {
		fmt.Printf("传输图片失败: %v\n", err)
	}
}

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
