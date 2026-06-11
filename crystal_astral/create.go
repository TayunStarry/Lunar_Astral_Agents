package main

import (
	"browser"
	"config"
	"context"
	"fmt"
	"io"
	"logger"
	"net/http"
	"os"
	"os/signal"
	"storage/module"
	"syscall"
	"time"
)

// proxyPrefixes 要代理的路径前缀
var proxyPrefixes = []string{"/v1/", "/generate", "/write/message", "/tts", "/tts/stream"}

// shouldProxy 判断是否需要代理路径
func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// copyBuffer 复制缓冲区内容到目标入参
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

// reloadPageParameters 重新加载页面参数
func reloadPageParameters() {
	*config.WebViewTitle = "星月智能 -> 轻量级-神经网络-本地部署方案"
	*config.WebViewWidth = 1500
	*config.WebViewHeight = 1050
}

// StartServer 启动服务器
func StartServer(port int, root http.FileSystem, name string) error {
	// 初始化统一数据库(SQL + 向量数据库基础)
	if err := module.InitUnifiedDB(*config.SQLDBPath, *config.VectorDBDir); err != nil {
		logger.Warn("CrystalAstral", "数据库初始化失败: %v (不影响服务启动)", err)
	}
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

	logger.Info("CrystalAstral", "%s 正运行在 http://localhost%s", name, serverAddr)
	reloadPageParameters()
	logger.SetDevMode(*config.Developer)
	go browser.OpenBrowser(fmt.Sprintf("http://localhost%s", serverAddr))

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("CrystalAstral", "%s 运行失败: %v", name, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("CrystalAstral", "%s 接收到中断信号，正在关闭...", name)
	case <-browser.WebViewClosed():
		logger.Info("CrystalAstral", "%s 检测到 WebView 关闭，正在关闭...", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("CrystalAstral", "%s 关闭失败: %v", name, err)
	}

	browser.CloseWebView()
	logger.Info("CrystalAstral", "%s 已成功关闭", name)

	return nil
}

// ServeHTTP 处理 HTTP 请求
// 根据路径判断是否需要通过代理转发
func (h *proxyAwareHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if h.shouldProxy(path) {
		h.proxy.ServeHTTP(w, r)
	} else {
		h.fs.ServeHTTP(w, r)
	}
}
