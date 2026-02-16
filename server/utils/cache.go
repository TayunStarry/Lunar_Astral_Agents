// utils 包提供了项目中常用的工具函数和中间件
package utils

// 导入必要的包
import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DisableCacheMiddleware 是一个 HTTP 中间件，用于禁用客户端缓存，同时设置 CORS 相关头信息。
func DisableCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// 设置 CORS 相关头信息，允许所有来源访问，支持多种 HTTP 方法和请求头
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			// 设置缓存相关头信息，禁用客户端缓存
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			// 处理 OPTIONS 请求，直接返回 200 状态码
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			// 调用下一个处理器
			next.ServeHTTP(w, r)
		},
	)
}

// CORSMiddleware 是一个 HTTP 中间件，用于设置 CORS 相关头信息。
// 对于 OPTIONS 请求，会直接返回 200 状态码。
// 参数 next 是需要处理的下一个 http.Handler。
// 返回一个新的 http.Handler。
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 设置 CORS 相关头信息，允许所有来源访问，支持多种 HTTP 方法和请求头
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理 OPTIONS 请求，直接返回 200 状态码
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 调用下一个处理器
		next.ServeHTTP(w, r)
	})
}

func PrintRequestBody(r *http.Request) {
	// 打印请求信息
	fmt.Printf("\n[HTTP Request] %s %s\n", r.Method, r.URL.Path)

	// 读取并打印请求体（仅针对非 OPTIONS 请求）
	if r.Method != "OPTIONS" && r.Body != nil {
		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			// 打印请求体
			fmt.Printf("[Request Body] %s\n", string(body))

			// 重置请求体，以便后续处理器可以读取
			r.Body = io.NopCloser(strings.NewReader(string(body)))
		}
	}
}
