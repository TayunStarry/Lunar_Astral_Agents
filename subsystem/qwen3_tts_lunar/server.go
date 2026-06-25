package main

import (
	"logger"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"qwen3_tts_lunar/module"
	"syscall"
	"time"
)

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
	{Path: "/tts", Handler: module.TTSHandler, Method: "POST", Description: "TTS语音合成服务"},
	{Path: "/upload/", Handler: module.UploadHandler, Method: "POST", Description: "参考音频上传"},
	{Path: "/health", Handler: module.HealthHandler, Method: "GET", Description: "健康检查"},
}

func registerHandlers() {
	httpMux = http.NewServeMux()

	for _, endpoint := range endpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
		logger.Info("QWEN-TTS", "注册端点: %s [%s] - %s", endpoint.Path, endpoint.Method, endpoint.Description)
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

	logger.Info("QWEN-TTS", "HTTP服务器启动于 http://%s", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("QWEN-TTS", "服务器启动失败: %v", err)
	}
}

func waitForServerReady(addr string, timeoutSeconds int) bool {
	logger.Info("QWEN-TTS", "等待服务就绪: %s (最多 %d 秒)", addr, timeoutSeconds)

	target := addr
	if target[0] == ':' {
		target = "127.0.0.1" + target
	}

	for i := 0; i < timeoutSeconds*10; i++ {
		conn, err := net.DialTimeout("tcp", target, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			logger.Info("QWEN-TTS", "服务已就绪")
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	logger.Error("QWEN-TTS", "服务就绪超时")
	return false
}

func setupSignalHandling() chan os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}

func shutdownServer() {
	uploadDir := "./local_data/audios"
	if _, err := os.Stat(uploadDir); err == nil {
		files, _ := filepath.Glob(filepath.Join(uploadDir, "ref_*.*"))
		for _, f := range files {
			os.Remove(f)
		}
		logger.Info("QWEN-TTS", "清理临时上传文件")
	}
}
