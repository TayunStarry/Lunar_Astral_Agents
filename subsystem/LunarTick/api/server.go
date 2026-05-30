package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"lunartick/engine"

	"logger"
)

type Server struct {
	eng      *engine.Engine
	httpSrv  *http.Server
	port     int
}

func NewServer(eng *engine.Engine, port int) *Server {
	return &Server{
		eng:  eng,
		port: port,
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()

	handler := NewAPIHandler(s.eng)
	handler.RegisterRoutes(mux)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.URL.Path != "/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Not Found"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"LunarTick API","version":"5.0"}`))
	})

	addr := fmt.Sprintf(":%d", s.port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("LunarAPI", "API 服务启动 -> http://localhost%s", addr)
	logger.Info("LunarAPI", "状态接口 [GET] -> http://localhost%s/api/status", addr)
	logger.Info("LunarAPI", "健康检查 [GET] -> http://localhost%s/api/health", addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("LunarAPI", "API 服务错误: %v", err)
		}
	}()

	s.eng.Start()
}

func (s *Server) Stop() {
	s.eng.Stop()

	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.httpSrv.Shutdown(ctx); err != nil {
			logger.Error("LunarAPI", "API 服务关闭错误: %v", err)
		}
	}

	logger.Info("LunarAPI", "API 服务已停止")
}

func (s *Server) Shutdown() {
	s.eng.Shutdown()

	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx)
	}
}

func (s *Server) Port() int {
	return s.port
}