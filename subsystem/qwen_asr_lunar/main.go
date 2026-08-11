package main

import (
	"LunarSubsystem/browser_client"
	"LunarSubsystem/general_config"
	"LunarSubsystem/general_logger"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

// reloadPageParameters 重新加载页面参数
func reloadPageParameters() {
	*config.WebViewTitle = "星月智能 -> 轻量级-神经网络-本地部署方案"
	*config.WebViewWidth = 648
	*config.WebViewHeight = 960
}

func main() {

	port := getEnv("PORT", fmt.Sprintf("%d", *config.ModelPort+1))
	modelDir := getEnv("MODEL_DIR", *config.AsrModel)

	logger.SetDevMode(*config.Developer, "local_data/documents/debug")

	logger.Info("ASREngine", "ASR Server 启动中...")
	logger.Info("ASREngine", "模型目录: %s", modelDir)
	logger.Info("ASREngine", "监听端口: %s", port)

	asr, err := New(modelDir)
	if err != nil {
		logger.Fatal("ASREngine", "ASR引擎初始化失败: %v", err)
	}
	defer asr.Close()

	logger.Info("ASREngine", "ASR引擎加载成功")

	handler := NewAsrHandler(asr, *config.LocalDir+"/audios")

	mux := http.NewServeMux()
	mux.Handle("/asr", handler)
	mux.Handle("/asr/", handler)
	mux.Handle("/health", handler)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	server := &http.Server{
		Addr:    ":" + port,
		Handler: logRequest(mux),
	}

	go func() {
		logger.Info("ASREngine", "HTTP服务监听于 http://localhost:%s", port)
		logger.Info("ASREngine", "ASR端点: http://localhost:%s/asr", port)
		logger.Info("ASREngine", "健康检查: http://localhost:%s/health", port)
		logger.Info("ASREngine", "测试页面: http://localhost:%s/", port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("ASREngine", "服务器错误: %v", err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%s", port)
	logger.Info("ASREngine", "打开浏览器: %s", url)
	reloadPageParameters()
	browser.OpenBrowser(url)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("ASREngine", "接收到信号，正在关闭服务器...")
	case <-browser.WebViewClosed():
		logger.Info("ASREngine", "WebView已关闭，正在关闭服务器...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("ASREngine", "服务器关闭错误: %v", err)
	}

	logger.Info("ASREngine", "服务器已停止")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("ASREngine", "[%s] %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
