package execute

import (
	"Lunar-Astral-Agents/server/config"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// GetModels 获取模型列表
func GetModels() []AgentModels {
	// 用于存储模型信息的切片
	models := []AgentModels{}
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
		models = append(models, AgentModels{ID: modelName, Object: "model", OwnedBy: "organization_owner"})
	}
	return models
}

// GetBusyResponse 返回“系统繁忙”响应（OpenAI 格式）
func GetBusyResponse() string {
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
	// 若编码失败，返回错误信息
	if err != nil {
		return "GGUF模块[ERROR] -> 生成响应失败"
	}
	return string(jsonData)
}

// ProcessAgentRequest 处理与模型相关的请求
func ProcessAgentRequest(modelName string) (string, error) {
	// 检查系统是否繁忙，如果已就绪的模型数量小于最大模型数量，返回系统繁忙响应
	if config.ModelReady < config.MaxModelAmount {
		return GetBusyResponse(), fmt.Errorf("system_busy")
	}
	// 队列控制
	queueMutex.Lock()
	// 检查当前处理状态和队列长度
	if currentProcessing >= 1 {
		// 如果队列长度超过最大值，返回系统繁忙
		if len(requestQueue) >= maxQueueLength {
			queueMutex.Unlock()
			return GetBusyResponse(), fmt.Errorf("system_busy")
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
	// 如果未能提取到模型名称，返回错误
	if modelName == "" {
		return "", fmt.Errorf("无法从请求中提取模型名称")
	}
	// 根据模型名称获取对应的端口号
	port, exists := GetModelPort(modelName)
	// 如果未找到对应的模型，返回错误
	if !exists {
		return "", fmt.Errorf("无法找到模型: %s", modelName)
	}
	// 打印日志，记录当前处理的模型及对应端口
	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("GGUF模块 -> 模型[ %s : %d ]", modelName, port)
	// 这里不直接代理，而是返回端口信息，由handlers处理代理
	return fmt.Sprintf("%d", port), nil
}

// ProcessAgentChatRequest 处理与模型相关的聊天请求
func ProcessAgentChatRequest(req AgentRequest) (AgentRequest, error) {
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
		return req, fmt.Errorf("请求中没有有效消息")
	}
	// 提取最新一条消息内容
	lastMsg := processedMessages[len(processedMessages)-1]
	// 检查最新消息内容是否为字符串类型
	if contentStr, ok := lastMsg.Content.(string); ok {
		latestContent = contentStr
	}
	// 获取动态系统提示词
	systemPrompt, err := GetDynamicSystemPrompt()
	// 检查是否获取系统提示词失败
	if err != nil {
		return req, fmt.Errorf("获取系统提示词失败: %w", err)
	}
	// 构建最终消息数组
	finalMessages := []Message{}
	// 添加系统提示词
	finalMessages = append(finalMessages, Message{Role: "system", Content: systemPrompt})
	// 获取知识消息
	knowledgeMessages, err := GetKnowledgeMessages(latestContent)
	// 检查是否获取知识消息失败
	if err != nil {
		return req, fmt.Errorf("获取知识消息失败: %w", err)
	}
	// 添加知识消息序列和最新消息序列
	finalMessages = append(finalMessages, append(knowledgeMessages, processedMessages...)...)
	// 构建新的请求体
	newReq := req
	newReq.Messages = finalMessages
	return newReq, nil
}

// GetModelPort 根据模型名称获取对应端口（加读锁）
func GetModelPort(modelName string) (int, bool) {
	// 加读锁，防止并发修改模型端口映射时出现数据竞争
	config.ModelMapMutex.RLock()
	// 函数结束时解锁，确保锁一定会被释放
	defer config.ModelMapMutex.RUnlock()
	// 从模型端口映射中查找指定模型的端口号
	port, exists := config.ModelPortMap[modelName]
	// 返回端口号和是否存在的标志
	return port, exists
}
