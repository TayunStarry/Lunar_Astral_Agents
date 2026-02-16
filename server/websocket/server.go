package websocket

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置包，用于获取配置信息
	"encoding/json"                     // 用于JSON编码和解码
	"fmt"                               // 用于格式化输出
	"log"                               // 用于日志记录
	"math/rand"                         // 用于生成随机数
	"net/http"                          // 用于HTTP请求和响应
	"net/http/httputil"
	"net/url"
	"slices" // 用于操作切片
	"time"   // 用于时间操作，如时间戳

	"golang.org/x/net/websocket" // 用于WebSocket连接
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
	// 启动定期清理过期请求的goroutine
	go cleanupExpiredRequests()
}

// BuildSimulatedServer 构建一个模拟的OpenAI V1格式服务器
func BuildSimulatedServer() *http.Server {
	// 创建独立的ServeMux实例
	mux := http.NewServeMux()
	// 定义HTTP处理器
	mux.HandleFunc("/v1/chat/", handleOpenAIRequest)
	mux.HandleFunc("/health", handleHealthCheck)
	// 添加WebSocket处理
	mux.Handle("/ws", websocket.Handler(handleWebSocket))
	// 添加默认代理处理
	mux.HandleFunc("/", handleProxyRequest)
	// 构建服务器地址
	serverAddr := fmt.Sprintf(":%s", serverState.config.Port)
	// 打印服务器端口
	log.Printf("Lunar模块[WebSocket] : 来源请求 [POST] -> http://localhost:%v/", serverState.config.Port)
	log.Printf("Lunar模块[WebSocket] : 持久连接 [GET] -> http://localhost:%v/ws", serverState.config.Port)
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

// 生成唯一请求ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}

// 清理过期请求
func cleanupExpiredRequests() {
	for {
		// 等待清理间隔
		time.Sleep(serverState.config.CleanupInterval)
		// 加锁以确保线程安全
		serverState.mutex.Lock()
		// 遍历所有请求上下文
		currentTime := time.Now()
		// 遍历所有请求上下文
		for id, ctx := range serverState.requests {
			// 检查请求是否过期
			if currentTime.Sub(ctx.CreatedAt) > serverState.config.RequestTimeout {
				// 清理过期请求
				close(ctx.ResponseChannel)
				delete(serverState.requests, id)
				log.Printf("Lunar模块[WebSocket] -> 清理过期请求: %s\n", id)
			}
		}
		// 解锁互斥锁
		serverState.mutex.Unlock()
	}
}

// 处理OpenAI V1格式请求
func handleOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	// 添加CORS头
	setCORSHeaders(w, r)
	// 处理预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// 处理POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "方法不被允许", http.StatusMethodNotAllowed)
		return
	}
	// 定义解析请求体的结构体
	var req OpenAIRequest
	// 解析请求体
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体无效", http.StatusBadRequest)
		return
	}
	// 生成请求ID
	requestID := generateRequestID()
	// 定义缓存消息的切片
	cachedMessages := []Message{}
	// 遍历缓存消息并过滤system消息
	for _, msg := range req.Messages {
		if msg.Role != "system" {
			cachedMessages = append(cachedMessages, msg)
		}
	}
	// 创建请求上下文
	responseChan := make(chan OpenAIResponse, 1)
	// 定义请求上下文
	requestCtx := &RequestContext{
		ID:              requestID,
		Messages:        cachedMessages,
		Tools:           req.Tools,
		ResponseChannel: responseChan,
		CreatedAt:       time.Now(),
	}
	// 加锁以确保线程安全
	serverState.mutex.Lock()
	// 检查请求数量是否超过限制
	if len(serverState.requests) >= serverState.config.MaxRequests {
		// 解锁互斥锁
		serverState.mutex.Unlock()
		// 超过最大请求数，返回错误
		http.Error(w, "请求数超过最大限制", http.StatusTooManyRequests)
		return
	}
	// 存储请求上下文
	serverState.requests[requestID] = requestCtx
	// 解锁互斥锁
	serverState.mutex.Unlock()
	// 通过WebSocket推送请求信息给客户端
	requestData := map[string]any{
		"id":       requestID,
		"messages": cachedMessages,
		"tools":    req.Tools,
	}
	// 推送请求信息给客户端
	err := pushMessageToWebSocket("new_request", requestData, requestID)
	// 检查推送是否成功
	if err != nil {
		log.Printf("Lunar模块[WebSocket] -> 推送请求信息失败: %v\n", err)
	}
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	// 等待AI响应 或 触发超时响应
	select {
	// 等待AI响应
	case response, ok := <-responseChan:
		// 检查通道是否已关闭
		if !ok {
			http.Error(w, "请求超时", http.StatusRequestTimeout)
			return
		}
		// 发送响应
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "编码响应失败", http.StatusInternalServerError)
			return
		}
		// 使用互斥锁 并 删除请求上下文
		serverState.mutex.Lock()
		delete(serverState.requests, requestID)
		serverState.mutex.Unlock()
	// 超时检测机制
	case <-time.After(serverState.config.RequestTimeout):
		// 超时处理
		http.Error(w, "请求超时", http.StatusRequestTimeout)
		// 使用互斥锁 并 删除请求上下文
		serverState.mutex.Lock()
		delete(serverState.requests, requestID)
		serverState.mutex.Unlock()
		// 日志记录超时请求
		log.Printf("Lunar模块[WebSocket] -> 请求超时，ID: %s\n", requestID)
	}
}

// 处理WebSocket消息，接收AI响应并返回给挂起请求
func handleAIResponseFromWebSocket(aiResponse OpenAIResponse, requestID string) error {
	// 使用互斥锁 并 查找请求上下文
	serverState.mutex.Lock()
	requestCtx, exists := serverState.requests[requestID]
	serverState.mutex.Unlock()
	// 检查请求上下文是否存在
	if !exists {
		return fmt.Errorf("Lunar模块[WebSocket] -> 未找到请求: %s", requestID)
	}
	// 发送消息到挂起的请求
	select {
	// 发送AI响应到挂起的请求
	case requestCtx.ResponseChannel <- aiResponse:
		// 使用互斥锁 并 删除请求上下文
		serverState.mutex.Lock()
		delete(serverState.requests, requestID)
		serverState.mutex.Unlock()

	// 通道已满或已关闭
	default:
		return fmt.Errorf("Lunar模块[WebSocket] -> 未能向请求发送响应: %s", requestID)
	}
	// 返回成功
	return nil
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

// handleWebSocket 负责维护与客户端的 WebSocket 长连接：
func handleWebSocket(ws *websocket.Conn) {
	// 设置新的 WebSocket 连接
	if !setupWebSocketConnection(ws) {
		return
	}
	// 延迟函数：在函数退出时确保连接资源被正确释放
	defer cleanupWebSocketConnection(ws)
	// 定义用于接收客户端消息的结构体
	var msg WSMessage
	// 主循环：持续读取客户端 JSON 消息，保持心跳与业务处理
	for {
		// 阻塞读取，若返回错误则判定为连接断开，跳出循环
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			break
		}
		// 处理接收到的消息
		handleWebSocketMessage(ws, msg)
	}
}

// setupWebSocketConnection 设置新的 WebSocket 连接，包括关闭旧连接和发送欢迎消息
func setupWebSocketConnection(ws *websocket.Conn) bool {
	// 临界区：确保并发情况下仅保留最新 WebSocket 连接
	serverState.wsMutex.Lock()
	// 若已存在旧连接，先主动关闭，避免资源泄漏
	if serverState.websocketConn != nil {
		// 关闭旧连接，释放资源
		if err := serverState.websocketConn.Close(); err != nil {
			log.Printf("Lunar模块[WebSocket] -> 关闭旧连接失败: %v\n", err)
		}
	}
	// 将新连接提升为“当前唯一合法连接”
	serverState.websocketConn = ws
	// 解锁WebSocket连接互斥锁，允许其他 goroutine 访问
	serverState.wsMutex.Unlock()
	// 连接建立后第一时间推送欢迎消息，告知客户端可开始通信
	welcomeMsg := WSMessage{
		Type: "connection_established",
		Data: "WebSocket连接已成功建立",
	}
	// 若欢迎消息发送失败，说明连接已不可用，直接返回并结束 handler
	if err := websocket.JSON.Send(ws, welcomeMsg); err != nil {
		log.Printf("Lunar模块[WebSocket] -> 发送欢迎消息失败: %v\n", err)
		return false
	}
	// 若欢迎消息发送成功，返回 true 表示连接已成功设置
	return true
}

// cleanupWebSocketConnection 清理 WebSocket 连接资源
func cleanupWebSocketConnection(ws *websocket.Conn) {
	// 临界区：确保并发情况下仅清理当前合法连接
	serverState.wsMutex.Lock()
	// 仅当当前连接仍是“合法连接”时才置空，防止并发覆盖
	if serverState.websocketConn == ws {
		serverState.websocketConn = nil
	}
	// 解锁WebSocket连接互斥锁，允许其他 goroutine 访问
	serverState.wsMutex.Unlock()
	// 关闭底层网络连接
	ws.Close()
}

// handleWebSocketMessage 处理接收到的 WebSocket 消息
func handleWebSocketMessage(ws *websocket.Conn, msg WSMessage) {
	// 仅处理业务约定的 "ai_response" 类型，且必须携带 RequestID
	if msg.Type == "ai_response" && msg.RequestID != "" {
		// 处理 AI 响应消息
		processAIResponse(ws, msg)
	}
}

// processAIResponse 处理 AI 响应消息的具体逻辑
func processAIResponse(ws *websocket.Conn, msg WSMessage) {
	// 将 interface{} 类型断言为 map，确保后续序列化/反序列化安全
	responseData, ok := msg.Data.(map[string]interface{})
	if !ok {
		log.Println("Lunar模块[WebSocket] -> AI响应数据格式无效")
		return
	}
	// 定义 OpenAI V1 响应结构体，用于反序列化 AI 响应数据
	var aiResponse OpenAIResponse
	// 借助 JSON 中转，把 map 转为结构体，降低手动赋值出错概率
	responseJSON, err := json.Marshal(responseData)
	// 若序列化失败，记录日志并返回
	if err != nil {
		log.Printf("Lunar模块[WebSocket] -> 序列化AI响应数据失败: %v\n", err)
		return
	}
	// 若反序列化失败，记录日志并返回
	if err := json.Unmarshal(responseJSON, &aiResponse); err != nil {
		log.Printf("Lunar模块[WebSocket] -> 解析AI响应数据失败: %v\n", err)
		return
	}
	// 将 AI 回答转发给对应的 HTTP 请求，并依据结果向客户端回执
	if err := handleAIResponseFromWebSocket(aiResponse, msg.RequestID); err != nil {
		log.Printf("Lunar模块[WebSocket] -> 处理AI响应失败: %v\n", err)
		// 转发失败时，及时通知客户端，便于上层业务感知
		sendErrorMessage(ws, err.Error(), msg.RequestID)
	} else {
		// 转发成功，回送「response_sent」确认，形成完整闭环
		sendSuccessMessage(ws, "AI响应已成功发送", msg.RequestID)
	}
}

// sendErrorMessage 发送错误消息到 WebSocket 客户端
func sendErrorMessage(ws *websocket.Conn, errorMsg string, requestID string) {
	// 定义错误消息结构体，符合业务协议
	msg := WSMessage{
		Type:      "error",
		Data:      errorMsg,
		RequestID: requestID,
	}
	// 尝试发送错误消息，若失败则记录日志
	if err := websocket.JSON.Send(ws, msg); err != nil {
		log.Printf("Lunar模块[WebSocket] -> 发送错误消息失败: %v\n", err)
	}
}

// sendSuccessMessage 发送成功消息到 WebSocket 客户端
func sendSuccessMessage(ws *websocket.Conn, successMsg string, requestID string) {
	// 定义成功消息结构体，符合业务协议
	msg := WSMessage{
		Type:      "response_sent",
		Data:      successMsg,
		RequestID: requestID,
	}
	// 尝试发送成功消息，若失败则记录日志
	if err := websocket.JSON.Send(ws, msg); err != nil {
		log.Printf("Lunar模块[WebSocket] -> 发送成功消息失败: %v\n", err)
	}
}

// 推送消息到WebSocket客户端
func pushMessageToWebSocket(messageType string, data interface{}, requestID string) error {
	// 加锁，确保并发安全，防止多个goroutine同时操作WebSocket连接
	serverState.wsMutex.Lock()
	// 函数退出时解锁，保证锁一定会被释放
	defer serverState.wsMutex.Unlock()
	// 检查当前是否存在有效的WebSocket连接
	if serverState.websocketConn == nil {
		// 若无活动连接，返回错误提示
		return fmt.Errorf("Lunar模块[WebSocket] -> 无活动WebSocket连接")
	}
	// 构造待发送的WebSocket消息结构体
	msg := WSMessage{
		Type:      messageType, // 消息类型
		Data:      data,        // 消息数据
		RequestID: requestID,   // 关联的请求ID
	}
	// 尝试通过WebSocket连接发送JSON格式的消息
	if err := websocket.JSON.Send(serverState.websocketConn, msg); err != nil {
		// 发送失败，可能连接已断开，将连接置空以便后续重连
		serverState.websocketConn = nil
		// 返回携带原始错误的格式化错误信息
		return fmt.Errorf("Lunar模块[WebSocket] -> 发送消息失败: %w", err)
	}
	// 消息发送成功，返回nil表示无错误
	return nil
}

// handleProxyRequest 处理代理请求，将其他路径的请求转发到 https 服务器
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
