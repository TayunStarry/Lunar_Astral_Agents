package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"CorePersonality/transport/ws"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// Start 装配路由并启动 HTTP 服务（非阻塞，监听成功后立即返回）。
// 单一监听端口：BasicPort（默认 36789）；allow_lan 时放开回环绑定限制。
func Start(roots StaticRoots) (*http.Server, error) {
	if ws.Instance == nil {
		return nil, errors.New("WebSocket 集线器未初始化")
	}

	// 默认仅绑定 127.0.0.1 回环地址：/ws 无鉴权且承载调试面，对局域网开放等于开放控制面。
	// 跨机部署时经 lunar_config.json 的 server.allow_lan 显式放开。
	bindHost := "127.0.0.1"
	if *GeneralConfig.AllowLAN {
		bindHost = ""
	}
	addr := fmt.Sprintf("%s:%d", bindHost, *GeneralConfig.BasicPort)
	// 对外展示用 URL：绑全部网卡时用 localhost，回环绑定时用 127.0.0.1
	urlHost := bindHost
	if urlHost == "" {
		urlHost = "localhost"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("监听端口 %s 失败(端口可能被占用): %w", addr, err)
	}

	httpMux = http.NewServeMux()

	// ==== WebSocket：唯一端点，两个前端共用 ====
	httpMux.HandleFunc("/ws", ws.Instance.HandleWebSocket)

	// ==== 静态前端资源 ====
	registerStatic(httpMux, roots)

	// ==== 统一路由注册表 ====
	if err := registerEndpoints(httpMux); err != nil {
		ln.Close()
		return nil, err
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(httpMux),
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			LoggerGeneral.Error("CorePersonality", "服务运行失败: %v", err)
		}
	}()

	LoggerGeneral.Info("CorePersonality", "核心人格服务正运行在 http://%s:%d", urlHost, *GeneralConfig.BasicPort)
	return srv, nil
}

// registerEndpoints 注册统一路由表，并在注册前做路径唯一性自检
// （http.ServeMux 对重复模式会 panic，此处提前给出可读错误）
func registerEndpoints(mux *http.ServeMux) error {
	seen := make(map[string]string, len(SystemEndpoints))
	for _, ep := range SystemEndpoints {
		if ep.Path == "" || ep.Handler == nil {
			return fmt.Errorf("路由注册表存在空路径或空处理器: %+v", ep)
		}
		if prev, ok := seen[ep.Path]; ok {
			return fmt.Errorf("路由路径重复注册: %s（%s）", ep.Path, prev)
		}
		seen[ep.Path] = ep.Description
		mux.HandleFunc(ep.Path, ep.Handler)
	}
	LoggerGeneral.Info("CorePersonality", "已注册 %d 个系统端点", len(SystemEndpoints))
	return nil
}

// Shutdown 优雅关闭 HTTP 服务
func Shutdown(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		LoggerGeneral.Error("CorePersonality", "服务关闭失败: %v", err)
	}
}

// corsMiddleware CORS 中间件（跨源预检直接放行，其余请求在响应后补齐头）
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			writeCORSHeaders(w)
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
		writeCORSHeaders(w)
	})
}

// writeCORSHeaders 写入统一 CORS 响应头
func writeCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
