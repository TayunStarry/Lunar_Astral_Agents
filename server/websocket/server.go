package websocket

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置包，用于获取配置信息
	"bytes"                             // 用于操作字节缓冲区
	"encoding/json"                     // 用于JSON编码和解码
	"fmt"                               // 用于格式化输出
	"io"                                // 用于I/O操作，如读取和写入
	"log"                               // 用于日志记录
	"net/http"                          // 用于HTTP请求和响应
	"net/http/httputil"                 // 用于HTTP请求和响应的调试工具
	"net/url"                           // 用于URL解析和操作
	"slices"                            // 用于操作切片
	"strings"                           // 用于字符串操作
	"time"                              // 用于时间操作，如时间戳
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
	mux.HandleFunc("/v1/chat/", handleAgentRequest)
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

// 处理OpenAI V1格式请求
func handleAgentRequest(w http.ResponseWriter, r *http.Request) {
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
	var req AgentRequest
	// 解析请求体
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体无效", http.StatusBadRequest)
		return
	}
	// 提取并预处理消息列表
	var processedMessages []Message
	// 过滤出非系统消息
	for _, msg := range req.Messages {
		if msg.Role != "system" {
			processedMessages = append(processedMessages, msg)
		}
	}
	// 提取最新一条消息用于向量化
	var latestContent string
	// 检查是否有有效消息
	if len(processedMessages) == 0 {
		http.Error(w, "请求中没有有效消息", http.StatusInternalServerError)
		return
	}
	// 提取最新一条消息内容
	lastMsg := processedMessages[len(processedMessages)-1]
	// 检查最新消息内容是否为字符串类型
	if contentStr, ok := lastMsg.Content.(string); ok {
		latestContent = contentStr
	}
	// 获取动态系统提示词
	systemPrompt, err := getDynamicSystemPrompt()
	// 检查是否获取系统提示词失败
	if err != nil {
		http.Error(w, "获取系统提示词失败", http.StatusInternalServerError)
		return
	}
	// 构建最终消息数组
	finalMessages := []Message{}
	// 添加系统提示词
	finalMessages = append(finalMessages, Message{Role: "system", Content: systemPrompt})
	// 获取知识消息
	knowledgeMessages, err := getKnowledgeMessages(latestContent)
	// 检查是否获取知识消息失败
	if err != nil {
		http.Error(w, "获取知识消息失败", http.StatusInternalServerError)
		return
	}
	// 添加知识消息序列和最新消息序列
	finalMessages = append(finalMessages, append(knowledgeMessages, processedMessages...)...)
	// 构建新的请求体
	newReq := req
	newReq.Messages = finalMessages
	// 序列化新的请求体
	jsonData, err := json.Marshal(newReq)
	// 检查是否序列化请求失败
	if err != nil {
		http.Error(w, "序列化请求失败", http.StatusInternalServerError)
		return
	}
	// 替换请求体
	r.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	// 更新Content-Length头
	r.ContentLength = int64(len(jsonData))
	// 使用handleProxyRequest转发请求
	handleProxyRequest(w, r)
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

// 获取动态系统提示词
func getDynamicSystemPrompt() (string, error) {
	// 构建文件读取URL
	fileURL := fmt.Sprintf("https://localhost:%d/read/resources/prompts/externalDialogue.md", *config.BasicPort)
	// 发送HTTP GET请求
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("获取系统提示词文件失败: %w", err)
	}
	defer resp.Body.Close()
	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取系统提示词文件失败，状态码: %d", resp.StatusCode)
	}
	// 读取文件内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取系统提示词文件内容失败: %w", err)
	}
	promptContent := string(body)
	// log.Printf("原始系统提示词: %s", promptContent)
	// 替换占位符
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	promptContent = strings.ReplaceAll(promptContent, "{current-time}", currentTime)
	promptContent = strings.ReplaceAll(promptContent, "{current-address}", "最终档案馆-[神之梦]档案室")
	return promptContent, nil
}

// 获取知识消息
func getKnowledgeMessages(latestContent string) ([]Message, error) {
	if latestContent == "" {
		return []Message{}, nil
	}
	// 获取嵌入向量
	queryVector, err := getEmbeddingVector(latestContent)
	if err != nil {
		return []Message{}, fmt.Errorf("获取嵌入向量失败: %w", err)
	}
	// 限制向量长度为256
	if len(queryVector) > 256 {
		queryVector = queryVector[:256]
	}
	// 查询知识库
	knowledgeEntries, err := queryKnowledgeBase(queryVector)
	if err != nil {
		return []Message{}, fmt.Errorf("查询知识库失败: %w", err)
	}
	// 构建知识消息
	var knowledgeMessages []Message
	for _, entry := range knowledgeEntries {
		if content, ok := entry.Content.(string); ok && content != "" {
			knowledgeMessages = append(knowledgeMessages, Message{
				Role:    "system",
				Content: content,
			})
		}
	}
	return knowledgeMessages, nil
}

// 获取嵌入向量
func getEmbeddingVector(text string) ([]float64, error) {
	// 构建嵌入请求
	embeddingReq := map[string]interface{}{
		"input": text,
		"model": "system-embedding",
	}
	// 构建请求URL
	embeddingURL := fmt.Sprintf("https://localhost:%d/v1/embeddings", *config.BasicPort)
	// 发送HTTP POST请求
	jsonData, err := json.Marshal(embeddingReq)
	if err != nil {
		return nil, fmt.Errorf("序列化嵌入请求失败: %w", err)
	}
	resp, err := http.Post(embeddingURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("发送嵌入请求失败: %w", err)
	}
	defer resp.Body.Close()
	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取嵌入向量失败，状态码: %d", resp.StatusCode)
	}
	// 解析响应
	var embeddingResp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}
	// 提取嵌入向量
	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("嵌入响应中无数据")
	}
	return embeddingResp.Data[0].Embedding, nil
}

// 查询知识库
func queryKnowledgeBase(queryVector []float64) ([]Message, error) {
	// 构建知识库查询请求
	knowledgeReq := map[string]interface{}{
		"filePath":    "knowledge/lunar_notes.json",
		"queryVector": queryVector,
		"topK":        10,
	}
	// 构建请求URL
	knowledgeURL := fmt.Sprintf("https://localhost:%d/knowledge/query", *config.BasicPort)
	// 发送HTTP POST请求
	jsonData, err := json.Marshal(knowledgeReq)
	if err != nil {
		return nil, fmt.Errorf("序列化知识库查询请求失败: %w", err)
	}
	resp, err := http.Post(knowledgeURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("发送知识库查询请求失败: %w", err)
	}
	defer resp.Body.Close()
	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询知识库失败，状态码: %d", resp.StatusCode)
	}
	// 解析响应
	var knowledgeEntries []Message
	if err := json.NewDecoder(resp.Body).Decode(&knowledgeEntries); err != nil {
		return nil, fmt.Errorf("解析知识库查询响应失败: %w", err)
	}
	return knowledgeEntries, nil
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
