package main

import (
	"browser"
	"config"
	"context"
	"fmt"
	"logger"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// StartServer 启动服务器
func StartServer(port int, root http.FileSystem, name string) error {
	httpMux := http.NewServeMux()
	fsHandler := http.FileServer(root)
	for _, endpoint := range SystemEndpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
	}
	proxy := getProxyHandler()
	proxyHandler := &proxyAwareHandler{
		fs:          fsHandler,
		proxy:       proxy,
		shouldProxy: shouldProxy,
	}
	httpMux.Handle("/", proxyHandler)

	serverAddr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: httpMux,
	}

	logger.Info("sd_lunar", "%s 正运行在 http://localhost%s", name, serverAddr)
	reloadPageParameters()
	logger.SetDevMode(*config.Developer)
	startTaskProcessor()
	go browser.OpenBrowser(fmt.Sprintf("http://localhost%s", serverAddr))

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("sd_lunar", "%s 运行失败: %v", name, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("sd_lunar", "%s 接收到中断信号，正在关闭...", name)
	case <-browser.WebViewClosed():
		logger.Info("sd_lunar", "%s 检测到 WebView 关闭，正在关闭...", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("sd_lunar", "%s 关闭失败: %v", name, err)
	}

	browser.CloseWebView()
	logger.Info("sd_lunar", "%s 已成功关闭", name)

	return nil
}

// ServeHTTP 处理 HTTP 请求
func (h *proxyAwareHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if h.shouldProxy(path) {
		h.proxy.ServeHTTP(w, r)
	} else {
		h.fs.ServeHTTP(w, r)
	}
}
