package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed static
var staticFiles embed.FS

const (
	defaultPort     = "35768"
	defaultModelDir = "C:\\Users\\196530\\Downloads\\Qwen3-ASR-0.6B-0"
	uploadDir       = "./uploads"
)

func main() {
	port := getEnv("PORT", defaultPort)
	modelDir := getEnv("MODEL_DIR", defaultModelDir)

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

	handler := NewAsrHandler(asr, uploadDir)

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
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
