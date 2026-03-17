// package handlers 定义处理程序所在的包
package handlers

// 导入所需的包
import (
	execute "Lunar-Astral-Agents/reasoning" // 导入执行模块，用于处理核心逻辑
	"bytes"                                 // 导入 bytes 包，用于操作字节切片
	"encoding/json"                         // 导入 json 包，用于 JSON 数据的编码和解码
	"io"                                    // 导入 io 包，用于操作输入输出流
	"net/http"                              // 导入 http 包，用于构建 HTTP 客户端和服务器
	"net/http/httputil"                     // 导入 httputil 包，用于反向代理
	"net/url"                               // 导入 url 包，用于解析和操作 URL
	"strconv"                               // 导入 strconv 包，用于字符串和数值之间的转换
	"strings"                               // 导入 strings 包，用于操作字符串
)

// ExtractModelName 从请求体或 URL 路径中提取模型名称
func ExtractModelName(r *http.Request) string {
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

// ProxyToPort 将请求反向代理到指定本地端口
func ProxyToPort(w http.ResponseWriter, r *http.Request, port int) {
	// 构造目标 URL，格式为 http://localhost:端口号
	targetURL := "http://localhost:" + strconv.Itoa(port)
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

// AgentModelsHandler 处理获取模型列表的请求, 返回本地模型列表。
func AgentModelsHandler(w http.ResponseWriter, r *http.Request) {
	// 调用 execute 模块获取模型列表
	models := execute.GetModels()
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
	// 从请求中提取模型名称
	modelName := ExtractModelName(r)
	// 调用 execute 模块处理请求
	result, err := execute.ProcessAgentRequest(modelName)
	if err != nil {
		if err.Error() == "system_busy" {
			// 返回系统繁忙响应
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(result))
		} else {
			http.Error(w, "GGUF模块[ERROR] -> "+err.Error(), http.StatusBadRequest)
		}
		return
	}

	// 解析端口号并进行代理
	port, err := strconv.Atoi(result)
	if err != nil {
		http.Error(w, "无效的端口号", http.StatusInternalServerError)
		return
	}

	// 调用本地的代理函数
	ProxyToPort(w, r, port)
}

// AgentChatHandler 处理与模型相关的请求, 返回模型输出。
func AgentChatHandler(w http.ResponseWriter, r *http.Request) {
	// 先读取整个请求体到内存中
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求体失败", http.StatusBadRequest)
		return
	}
	// 关闭原始请求体
	r.Body.Close()
	// 定义解析请求体的结构体
	var req execute.AgentRequest
	// 从读取到的字节中解析请求体
	if err = json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, "请求体无效", http.StatusBadRequest)
		return
	}
	// 恢复原始请求体，确保后续处理可以读取
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))
	// 调用 execute 模块处理聊天请求
	processedReq, err := execute.ProcessAgentChatRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// 序列化新的请求体
	jsonData, err := json.Marshal(processedReq)
	if err != nil {
		http.Error(w, "序列化请求失败", http.StatusInternalServerError)
		return
	}
	// 替换请求体
	r.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	// 更新Content-Length头
	r.ContentLength = int64(len(jsonData))
	// 调用 AgentHandler 处理请求
	AgentHandler(w, r)
}
