package lunar_window

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lunar_window/browser"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var proxyPrefixes = []string{"/delete/", "/file_list/", "/download/", "/archive", "/save", "/read/", "/generate", "/database/", "/v1/", "/load/"}

func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

type LoadApplicationRequest struct {
	Path string `json:"path"`
}

type LoadApplicationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

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

func getRandomBackgroundImage() (string, error) {
	fs := GetResourceFS()
	file, err := fs.Open("/")
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	var files []string
	if stat.IsDir() {
		entries, err := file.Readdir(-1)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "page_background") {
				files = append(files, entry.Name())
			}
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("未找到 page_background 开头的图片文件")
	}
	randomIndex := rand.Intn(len(files))
	return files[randomIndex], nil
}

func serveRandomBackground(w http.ResponseWriter, _ *http.Request) {
	filename, err := getRandomBackgroundImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	fs := GetResourceFS()
	file, err := fs.Open("/" + filename)
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

func copyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				er = nil
			}
			return written, er
		}
	}
}

// StartServer 启动文件浏览器服务
func StartServer(port int, root http.FileSystem, name string) error {
	// 创建嵌入式文件系统的处理器
	fsHandler := http.FileServer(root)
	// 创建反向代理，转发到本地 36789 端口
	proxyURL, err := url.Parse("http://localhost:36789")
	if err != nil {
		return fmt.Errorf("解析代理 URL 失败: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	// 主处理器，处理所有请求
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/background" && r.Method == "GET" {
			serveRandomBackground(w, r)
		} else if path == "/load/application" && r.Method == "POST" {
			loadApplicationHandler(w, r)
		} else if shouldProxy(path) {
			proxy.ServeHTTP(w, r)
		} else {
			fsHandler.ServeHTTP(w, r)
		}
	})

	// 启动服务器
	serverAddr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: handler,
	}

	fmt.Printf("%s 正运行在 http://localhost%s\n", name, serverAddr)

	// 打开浏览器访问前端
	go browser.OpenBrowser(fmt.Sprintf("http://localhost%s", serverAddr))

	// 启动服务器（非阻塞）
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("%s 运行失败: %v\n", name, err)
		}
	}()

	// 等待中断信号或 webview 关闭信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		fmt.Printf("%s 接收到中断信号，正在关闭...\n", name)
	case <-browser.WebViewClosed():
		fmt.Printf("%s 检测到 WebView 关闭，正在关闭...\n", name)
	}

	// 设置 5 秒的超时时间来关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("%s 关闭失败: %v\n", name, err)
	}

	// 关闭浏览器
	browser.CloseWebView()
	fmt.Printf("%s 已成功关闭\n", name)

	return nil
}
