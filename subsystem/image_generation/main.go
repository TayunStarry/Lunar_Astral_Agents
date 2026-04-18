package main

import (
	"subsystem/component"
	"subsystem/component/browser"

	//"LunarCore/server"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
)

var proxyPrefixes = []string{"/delete/", "/file_list/", "/download/", "/archive", "/save", "/read/", "/generate"}

func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// StartServer 启动文件浏览器服务
func StartServer(port int) error {
	// 创建嵌入式文件系统的处理器
	fsHandler := http.FileServer(component.Gethierarchy())

	// 创建反向代理，转发到本地 36789 端口
	proxyURL, err := url.Parse("http://localhost:36789")
	if err != nil {
		return fmt.Errorf("解析代理 URL 失败: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	// 主处理器，处理所有请求
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if shouldProxy(path) {
			proxy.ServeHTTP(w, r)
		} else {
			fsHandler.ServeHTTP(w, r)
		}
	})

	// 启动服务器
	serverAddr := fmt.Sprintf(":%d", port)
	fmt.Printf("文件浏览器服务启动在 http://localhost%s\n", serverAddr)

	// 打开浏览器访问前端
	go browser.OpenBrowser(fmt.Sprintf("http://localhost%s", serverAddr))

	return http.ListenAndServe(serverAddr, nil)
}

func main() {
	// 生成10000~40000之间的随机端口
	port := rand.Intn(30001) + 10000
	// 启动服务
	if err := StartServer(port); err != nil {
		fmt.Printf("启动服务失败: %v\n", err)
	}
}
