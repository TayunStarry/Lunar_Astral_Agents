package websocket

// 导入必要的包
import (
	config "Lunar-Astral-Agents/parameter" // 引入配置模块，用于获取模型路径等配置
	"encoding/json"                        // 用于JSON编码和解码
	"fmt"                                  // 用于格式化输出
	"log"                                  // 用于日志记录
	"net/http"                             // 用于HTTP请求和响应
	"net/http/httputil"                    // 用于HTTP请求和响应的调试工具
	"net/url"                              // 用于URL解析和操作
	"slices"                               // 用于操作切片
	"time"                                 // 用于时间操作，如时间戳
)

// 全局服务器状态
var serverState *ServerState

// init 初始化服务器状态
func init() {
	serverState = &ServerState{
		requests: make(map[string]*RequestContext),
		config: ServerConfig{
			Port:               fmt.Sprintf("%d", *config.BasicPort+5),
			CORSAllowedOrigins: []string{fmt.Sprintf("https://localhost:%d", *config.BasicPort)},
			RequestTimeout:     2 * time.Minute,
			MaxRequests:        100,
			CleanupInterval:    5 * time.Minute,
		},
	}
}

// BuildSimulatedServer 构建一个模拟的OpenAI V1格式服务器
func BuildSimulatedServer() *http.Server {
	// 创建独立的ServeMux实例
	mux := http.NewServeMux()
	// 定义HTTP处理器
	mux.HandleFunc("/health", handleHealthCheck)
	// 添加默认代理处理
	mux.HandleFunc("/", handleProxyRequest)
	// 构建服务器地址
	serverAddr := fmt.Sprintf(":%s", serverState.config.Port)
	// 打印服务器端口
	log.Printf("Lunar模块[WebSocket] : 代理请求 [POST] -> http://localhost:%v/", serverState.config.Port)
	log.Printf("Lunar模块[WebSocket] : 健康检查 [GET] -> http://localhost:%v/health", serverState.config.Port)
	// 创建服务器实例
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}
	// 启动服务器
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Lunar模块[WebSocket][ERROR] -> 服务器启动失败: %v\n", err)
		}
	}()
	// 返回服务器实例
	return server
}

// handleHealthCheck 处理健康检查请求
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// 添加CORS头，允许跨域访问
	setCORSHeaders(w, r)
	// 如果是预检请求（OPTIONS），直接返回200 OK
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// 只允许GET方法，否则返回405 Method Not Allowed
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 使用互斥锁 并 读取挂起请求数量
	serverState.mutex.RLock()
	pendingRequests := len(serverState.requests)
	serverState.mutex.RUnlock()
	// 构造健康检查响应数据
	healthStatus := map[string]any{
		"status":           "healthy",                      // 服务状态：健康
		"timestamp":        time.Now(),                     // 当前时间戳
		"pending_requests": pendingRequests,                // 当前挂起的请求数
		"max_requests":     serverState.config.MaxRequests, // 最大允许请求数
		"port":             serverState.config.Port,        // 服务监听端口
	}
	// 设置响应头，指定返回JSON格式
	w.Header().Set("Content-Type", "application/json")
	// 将健康状态编码为JSON并写入响应体
	if err := json.NewEncoder(w).Encode(healthStatus); err != nil {
		// 编码失败，返回500内部服务器错误
		http.Error(w, "编码响应失败", http.StatusInternalServerError)
		return
	}
}

// setCORSHeaders 设置跨域资源共享（CORS）响应头
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	// 从请求头中获取来源（Origin）
	origin := r.Header.Get("Origin")
	// 检查该来源是否在服务器允许的白名单中
	allowed := slices.Contains(serverState.config.CORSAllowedOrigins, origin)
	// 如果来源被允许，则将其写回响应头，允许该来源跨域访问
	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		// 否则使用白名单中的第一个来源作为默认值，避免暴露空值
		w.Header().Set("Access-Control-Allow-Origin", serverState.config.CORSAllowedOrigins[0])
	}
	// 设置允许的HTTP方法：POST、GET、OPTIONS
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	// 设置允许的请求头：Content-Type、Authorization
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	// 设置预检请求（OPTIONS）的缓存时间，单位为秒，86400秒即24小时
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// 处理代理请求，将其他路径的请求转发到 https 服务器
func handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	// 添加CORS头
	setCORSHeaders(w, r)
	// 处理预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// 目标服务器地址 - 构建本地HTTP服务器的URL
	targetURL := fmt.Sprintf("https://localhost:%d", *config.BasicPort)
	// 解析目标URL - 将字符串URL转换为url.URL对象
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "目标URL解析失败", http.StatusInternalServerError)
		return
	}
	// 创建反向代理 - 使用标准库创建单主机反向代理
	proxy := httputil.NewSingleHostReverseProxy(target)
	// 自定义请求处理器 - 配置请求转发规则
	proxy.Director = func(req *http.Request) {
		// 保存原始URL用于日志 - 记录客户端请求的原始URL
		originalURL := req.URL.String()
		// 设置协议为后端服务器的协议
		req.URL.Scheme = target.Scheme
		// 设置主机为后端服务器的主机
		req.URL.Host = target.Host
		// 设置Host头为后端服务器的主机
		req.Host = target.Host
		// 指示原始请求使用HTTPS
		req.Header.Set("X-Forwarded-Proto", "https")
		// 指示原始请求的端口
		req.Header.Set("X-Forwarded-Port", fmt.Sprintf("%d", *config.BasicPort+5))
		// 开发模式日志 - 记录请求转发详情
		if *config.DevMode {
			log.Printf("转发请求: %s %s -> %s%s", req.Method, originalURL, targetURL, req.URL.Path)
		}
	}
	// 自定义错误处理器 - 处理代理过程中的错误
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// 记录错误日志 - 输出详细的错误信息
		log.Printf("代理错误: %v", err)
		// 设置错误响应头 - 返回JSON格式的错误信息
		w.Header().Set("Content-Type", "application/json")
		// 使用502状态码表示网关错误
		w.WriteHeader(http.StatusBadGateway)
		// 写入错误响应体 - 提供友好的错误信息
		errorMsg := fmt.Sprintf(`{"error": "无法连接到后端服务器", "message": "%s"}`, err.Error())
		w.Write([]byte(errorMsg))
	}
	// 执行代理请求 - 调用反向代理处理请求
	proxy.ServeHTTP(w, r)
}
