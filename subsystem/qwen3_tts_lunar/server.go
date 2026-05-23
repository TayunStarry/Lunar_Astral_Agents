package main

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"qwen3_tts_lunar/module"
	"syscall"
	"time"
)

//go:embed client/*
var clientFiles embed.FS

var (
	httpMux    *http.ServeMux
	httpServer *http.Server
)

type Endpoint struct {
	Path        string
	Handler     func(http.ResponseWriter, *http.Request)
	Method      string
	Description string
}

var endpoints = []Endpoint{
	{Path: "/tts/", Handler: module.TTSHandler, Method: "POST", Description: "TTS语音合成服务"},
	{Path: "/tts/stream", Handler: module.TTSStreamHandler, Method: "GET", Description: "TTS流式合成服务"},
	{Path: "/upload/", Handler: module.UploadHandler, Method: "POST", Description: "参考音频上传"},
	{Path: "/health", Handler: module.HealthHandler, Method: "GET", Description: "健康检查"},
}

func registerHandlers() {
	httpMux = http.NewServeMux()

	clientFS, err := fs.Sub(clientFiles, "client")
	if err == nil {
		fileServer := http.FileServer(http.FS(clientFS))
		httpMux.Handle("/", http.StripPrefix("/", fileServer))
		log.Println("[Server] 使用嵌入的客户端文件")
	} else {
		assetsDir := "./client"
		if _, err := os.Stat(assetsDir); err == nil {
			fileServer := http.FileServer(http.Dir(assetsDir))
			httpMux.Handle("/", http.StripPrefix("/", fileServer))
			log.Println("[Server] 使用本地文件系统客户端文件")
		} else {
			log.Println("[Server] 警告: 未找到客户端文件目录")
			httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, "<html><body><h1>404 Not Found</h1><p>客户端文件未找到</p></body></html>")
			})
		}
	}

	for _, endpoint := range endpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
		log.Printf("[Server] 注册端点: %s [%s] - %s", endpoint.Path, endpoint.Method, endpoint.Description)
	}
}

func startServer(addr string) {
	registerHandlers()

	httpServer = &http.Server{
		Addr:         addr,
		Handler:      httpMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("[Server] HTTP服务器启动于 http://%s", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[Server] 服务器启动失败: %v", err)
	}
}

func waitForServerReady(addr string, timeoutSeconds int) bool {
	log.Printf("[Server] 等待服务就绪: %s (最多 %d 秒)", addr, timeoutSeconds)

	target := addr
	if target[0] == ':' {
		target = "127.0.0.1" + target
	}

	for i := 0; i < timeoutSeconds*10; i++ {
		conn, err := net.DialTimeout("tcp", target, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			log.Println("[Server] 服务已就绪")
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("[Server] 服务就绪超时")
	return false
}

func setupSignalHandling() chan os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}

func waitForShutdown(quit chan os.Signal) {
	<-quit
	log.Println("[Server] 正在关闭...")

	if httpServer != nil {
		shutdownServer()
	}

	log.Println("[Server] 已安全关闭")
}

func shutdownServer() {
	uploadDir := "./local_data/audios"
	if _, err := os.Stat(uploadDir); err == nil {
		files, _ := filepath.Glob(filepath.Join(uploadDir, "ref_*.*"))
		for _, f := range files {
			os.Remove(f)
		}
		log.Println("[Server] 清理临时上传文件")
	}
}
