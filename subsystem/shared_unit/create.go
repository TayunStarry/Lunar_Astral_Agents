package shared_unit

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"shared_unit/browser"
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
	fmt.Printf("%s 正运行在 http://localhost%s\n", name, serverAddr)

	// 打开浏览器访问前端
	go browser.OpenBrowser(fmt.Sprintf("http://localhost%s", serverAddr))

	return http.ListenAndServe(serverAddr, nil)
}
