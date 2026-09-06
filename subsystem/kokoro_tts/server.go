package main

import (
	"LunarSubsystem/Kokoro-TTS/module"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

var (
	httpMux    *http.ServeMux
	httpServer *http.Server
)

// Endpoint 接口定义
type Endpoint struct {
	Path        string
	Handler     func(http.ResponseWriter, *http.Request)
	Method      string
	Description string
}

var endpoints = []Endpoint{
	{Path: "/tts", Handler: module.TTSHandler, Method: "POST", Description: "语音合成服务"},
	{Path: "/voices", Handler: module.VoicesHandler, Method: "GET", Description: "音色列表"},
	{Path: "/dict", Handler: module.DictHandler, Method: "GET/POST/DELETE", Description: "读音词典管理"},
	{Path: "/dict/guess", Handler: module.GuessDictHandler, Method: "GET", Description: "读音查询"},
	{Path: "/health", Handler: module.HealthHandler, Method: "GET", Description: "健康检查"},
}

func registerHandlers() {
	httpMux = http.NewServeMux()

	for _, endpoint := range endpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
		LoggerGeneral.Info("KOKORO-TTS", "注册端点: %s [%s] - %s", endpoint.Path, endpoint.Method, endpoint.Description)
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

	LoggerGeneral.Info("KOKORO-TTS", "HTTP服务器启动于 http://%s", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		LoggerGeneral.Fatal("KOKORO-TTS", "服务器启动失败: %v", err)
	}
}

func waitForServerReady(addr string, timeoutSeconds int) bool {
	LoggerGeneral.Info("KOKORO-TTS", "等待服务就绪: %s (最多 %d 秒)", addr, timeoutSeconds)

	target := addr
	if target[0] == ':' {
		target = "127.0.0.1" + target
	}

	for i := 0; i < timeoutSeconds*10; i++ {
		conn, err := net.DialTimeout("tcp", target, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			LoggerGeneral.Info("KOKORO-TTS", "服务已就绪")
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	LoggerGeneral.Error("KOKORO-TTS", "服务就绪超时")
	return false
}

func setupSignalHandling() chan os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}
