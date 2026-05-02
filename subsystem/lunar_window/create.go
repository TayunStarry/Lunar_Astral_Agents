package shared_unit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"lunar_window/browser"
	"syscall"
	"time"
)

var proxyPrefixes = []string{"/delete/", "/file_list/", "/download/", "/archive", "/save", "/read/", "/generate", "/database"}

func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
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
		if shouldProxy(path) {
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
