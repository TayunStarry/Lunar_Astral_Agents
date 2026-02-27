// package handlers 定义处理程序所在的包
package handlers

// 导入所需的包
import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置包，用于获取配置信息
	"bytes"                             // 导入 bytes 包，用于处理字节切片
	"encoding/json"                     // 导入 json 包，用于 JSON 数据的编码和解码
	"fmt"                               // 导入 fmt 包，用于格式化输入输出
	"io"                                // 导入 io 包，提供基本的 I/O 操作接口
	"log"                               // 导入 log 包，用于日志记录
	"net/http"                          // 导入 http 包，用于构建 HTTP 客户端和服务器
	"net/http/httputil"                 // 导入 httputil 包，提供 HTTP 协议的实用工具
	"net/url"                           // 导入 url 包，用于解析和处理 URL
	"strings"                           // 导入 strings 包，用于处理字符串
	"sync"                              // 导入 sync 包，用于同步操作
	"time"                              // 导入 time 包，用于处理时间
)

// 请求队列相关变量
var (
	// 最大队列长度
	maxQueueLength = 3
	// 当前正在处理的请求数
	currentProcessing int
	// 请求队列
	requestQueue []chan struct{}
	// 队列锁
	queueMutex sync.Mutex
)

// AgentModelsHandler 处理获取模型列表的请求, 返回本地模型列表。
func AgentModelsHandler(w http.ResponseWriter, r *http.Request) {
	// 用于存储模型信息的切片
	models := []map[string]any{}
	// 加读锁，防止并发修改模型端口映射
	config.ModelMapMutex.RLock()
	// 函数结束时解锁
	defer config.ModelMapMutex.RUnlock()
	// 存储模型名称的切片
	var modelNames []string
	// 遍历模型端口映射，获取所有模型名称
	for modelName := range config.ModelPortMap {
		modelNames = append(modelNames, modelName)
	}
	// 遍历模型名称，构造模型信息
	for _, modelName := range modelNames {
		models = append(models, map[string]any{
			"id":       modelName,
			"object":   "model",
			"owned_by": "organization_owner",
		})
	}
	// 构造响应数据
	response := map[string]any{
		"object": "list",
		"data":   models,
	}
	// 设置响应头，指定返回数据为 JSON 格式
	w.Header().Set("Content-Type", "application/json")
	// 将响应数据编码为 JSON 并写入响应
	json.NewEncoder(w).Encode(response)
}

// AgentHandler 处理与模型相关的请求, 返回模型输出。
func AgentHandler(w http.ResponseWriter, r *http.Request) {
	// 检查系统是否繁忙，如果已就绪的模型数量小于最大模型数量，返回系统繁忙响应
	if config.ModelReady < config.MaxModelAmount {
		serveBusyResponse(w)
		return
	}

	// 队列控制
	queueMutex.Lock()
	// 检查当前处理状态和队列长度
	if currentProcessing >= 1 {
		// 如果队列长度超过最大值，返回系统繁忙
		if len(requestQueue) >= maxQueueLength {
			queueMutex.Unlock()
			serveBusyResponse(w)
			return
		}
		// 创建一个通道用于等待
		waitChan := make(chan struct{})
		// 将通道加入队列
		requestQueue = append(requestQueue, waitChan)
		queueMutex.Unlock()

		// 等待前面的请求处理完成
		<-waitChan
	} else {
		// 标记当前正在处理请求
		currentProcessing = 1
		queueMutex.Unlock()
	}

	// 处理完成后释放资源
	defer func() {
		queueMutex.Lock()
		defer queueMutex.Unlock()
		// 标记处理完成
		currentProcessing = 0
		// 如果队列不为空，通知下一个请求
		if len(requestQueue) > 0 {
			nextChan := requestQueue[0]
			requestQueue = requestQueue[1:]
			close(nextChan)
		}
	}()

	// 从请求中提取模型名称
	modelName := extractModelName(r)
	// 如果未能提取到模型名称，返回 400 错误
	if modelName == "" {
		http.Error(w, "GGUF模块[ERROR] -> 无法从请求中提取模型名称", http.StatusBadRequest)
		return
	}
	// 根据模型名称获取对应的端口号
	port, exists := getModelPort(modelName)
	// 如果未找到对应的模型，返回 404 错误
	if !exists {
		http.Error(w, "GGUF模块[ERROR] -> 无法找到模型: "+modelName, http.StatusNotFound)
		return
	}
	// 打印日志，记录当前处理的模型及对应端口
	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("GGUF模块 -> 模型[ %s : %d ]", modelName, port)
	// 将请求反向代理到模型对应的端口
	proxyToPort(w, r, port)
}

// serveBusyResponse 返回“系统繁忙”响应（OpenAI 格式）
func serveBusyResponse(w http.ResponseWriter) {
	// 构造系统繁忙的响应数据（OpenAI 格式）
	response := map[string]any{
		"choices": []map[string]any{
			{
				// 完成原因设为 "stop"，表示响应已完成
				"finish_reason": "stop",
				// 选择索引设为 0
				"index": 0,
				// 消息内容，提示用户系统繁忙
				"message": map[string]any{
					"role":    "assistant",
					"content": "请稍等哦, 月华现在正忙呢~~",
				},
			},
		},
		// 响应创建时间戳
		"created": time.Now().Unix(),
		// 响应 ID，添加当前纳秒时间戳确保唯一性
		"id": "chatcmpl-busy-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		// 模型名称，标记为系统繁忙模型
		"model": "system-busy",
		// 系统指纹，添加当前纳秒时间戳确保唯一性
		"system_fingerprint": "busy-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		// 对象类型，标记为聊天完成
		"object": "chat.completion",
	}

	// 将响应数据编码为 JSON 格式
	jsonData, err := json.Marshal(response)
	// 若编码失败，返回 500 错误并提示生成响应失败
	if err != nil {
		http.Error(w, "GGUF模块[ERROR] -> 生成响应失败", http.StatusInternalServerError)
		return
	}

	// 设置响应头，指定内容类型为 JSON 格式，使用 UTF-8 编码
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// 写入 HTTP 状态码 200，表示请求成功
	w.WriteHeader(http.StatusOK)
	// 直接写入 JSON 数据
	w.Write(jsonData)
}

// extractModelName 从请求体或 URL 路径中提取模型名称
func extractModelName(r *http.Request) string {
	// 用于存储从请求中提取的模型名称
	var modelName string
	// 用于存储请求体的字节数据，方便后续读取和恢复请求体
	var bodyBytes []byte
	// 检查请求方法是否为 POST，如果是则尝试从请求体中提取模型名称
	if r.Method == "POST" {
		var err error
		// 读取请求体的所有内容
		bodyBytes, err = io.ReadAll(r.Body)
		// 若读取失败，返回空字符串
		if err != nil {
			return ""
		}
		// 关闭请求体
		r.Body.Close()
		// 用于存储反序列化后的请求体数据
		var requestBody map[string]any
		// 将请求体内容反序列化为 map
		if err := json.Unmarshal(bodyBytes, &requestBody); err == nil {
			// 尝试从请求体中获取 "model" 字段的值
			if model, ok := requestBody["model"].(string); ok {
				modelName = model
			}
		}
		// 恢复请求体，确保后续处理可以再次读取请求体内容
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		// 更新请求体长度
		r.ContentLength = int64(len(bodyBytes))
	}
	// 如果从请求体中未提取到模型名称，则尝试从 URL 路径中提取
	if modelName == "" {
		// 将 URL 路径按 "/" 分割成多个部分
		pathParts := strings.Split(r.URL.Path, "/")
		// 若分割后的路径部分数量大于 2，则取第 3 部分作为模型名称
		if len(pathParts) > 2 {
			modelName = pathParts[2]
		}
	}
	return modelName
}

// getModelPort 根据模型名称获取对应端口（加读锁）
func getModelPort(modelName string) (int, bool) {
	// 加读锁，防止并发修改模型端口映射时出现数据竞争
	config.ModelMapMutex.RLock()
	// 函数结束时解锁，确保锁一定会被释放
	defer config.ModelMapMutex.RUnlock()
	// 从模型端口映射中查找指定模型的端口号
	port, exists := config.ModelPortMap[modelName]
	// 返回端口号和是否存在的标志
	return port, exists
}

// proxyToPort 将请求反向代理到指定本地端口
func proxyToPort(w http.ResponseWriter, r *http.Request, port int) {
	// 构造目标 URL，格式为 http://localhost:端口号
	targetURL := fmt.Sprintf("http://localhost:%d", port)
	// 解析目标 URL，将字符串形式的 URL 转换为 url.URL 结构体
	target, err := url.Parse(targetURL)
	// 若解析失败，返回 500 错误并提示解析目标 URL 失败
	if err != nil {
		http.Error(w, "GGUF模块[ERROR] -> 解析目标 URL 失败", http.StatusInternalServerError)
		return
	}
	// 创建一个单主机反向代理，将请求转发到解析后的目标 URL
	proxy := httputil.NewSingleHostReverseProxy(target)
	// 执行反向代理，将请求转发到目标地址并将响应返回给客户端
	proxy.ServeHTTP(w, r)
}
