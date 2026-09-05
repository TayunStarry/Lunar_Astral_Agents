package main

// ==== LTP3 权限密钥生成器入口 ====
// 根开发者在 Web 前端选择权限、导入插件脚本（或填写插件目录），
// 生成 permissions.key 写入插件包，供 LTP3 引擎加载时校验授权。

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

func main() {
	flag.Parse()
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer)

	// webview 窗口尺寸（默认 648 太小，密钥生成界面按较大尺寸布局）
	*GeneralConfig.WebViewTitle = "LTP3 权限密钥生成器"
	*GeneralConfig.WebViewWidth = 1320
	*GeneralConfig.WebViewHeight = 900
	*GeneralConfig.WebViewMinWidth = 960
	*GeneralConfig.WebViewMinHeight = 680
	*GeneralConfig.WebViewResizable = true

	port := *serverPort
	if port == 0 {
		port = randomPort()
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/perms", permsHandler)
	mux.HandleFunc("/api/gen", genHandler)
	mux.HandleFunc("/api/verify", verifyHandler)

	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		LoggerGeneral.Info("LTP3Keygen", "密钥生成器服务已启动: http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			LoggerGeneral.Error("LTP3Keygen", "服务运行失败: %v", err)
		}
	}()

	url := fmt.Sprintf("http://%s", addr)
	BrowserClient.OpenBrowser(url)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		LoggerGeneral.Info("LTP3Keygen", "收到中断信号，正在关闭...")
	case <-BrowserClient.WebViewClosed():
		LoggerGeneral.Info("LTP3Keygen", "窗口已关闭，正在退出...")
	}
	BrowserClient.CloseWebView()
}

// randomPort 生成 10000-40000 的随机端口。
func randomPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 18000
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}