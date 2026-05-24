package main

import (
	"browser"
	"config"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
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

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("Qwen ASR Server starting...")
	log.Printf("Model directory: %s", modelDir)
	log.Printf("Server will listen on port: %s", port)

	asr, err := New(modelDir)
	if err != nil {
		log.Fatalf("Failed to initialize ASR engine: %v", err)
	}
	defer asr.Close()

	log.Println("ASR engine loaded successfully")

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
		log.Printf("HTTP server listening on http://localhost:%s", port)
		log.Printf("ASR endpoint: http://localhost:%s/asr", port)
		log.Printf("Health check: http://localhost:%s/health", port)
		log.Printf("Test page: http://localhost:%s/", port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%s", port)
	log.Printf("Opening browser: %s", url)
	reloadPageParameters()
	browser.OpenBrowser(url)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("Signal received, shutting down server...")
	case <-browser.WebViewClosed():
		log.Println("WebView closed, shutting down server...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
