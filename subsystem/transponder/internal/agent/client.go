package agent

import (
	"bytes"                          // 用于处理字节缓冲区
	"encoding/json"                  // 用于JSON编码和解码
	"fmt"                            // 格式化输出
	"io"                             // 用于IO操作
	"log"                            // 日志记录
	"mime/multipart"                 // 用于处理multipart表单
	"net/http"                       // HTTP客户端
	"regexp"                         // 用于正则表达式操作
	"strings"                        // 字符串操作
	"time"                           // 时间操作
	"transponder/internal/processor" // 核心处理器
	"transponder/internal/setup"     // 配置文件
)

// NewClient 创建新的OpenAI客户端
func NewClient(cfg *setup.Config) *Client {
	return &Client{
		agentURL: cfg.OpenAIAPIUrl,
		token:    cfg.OpenAIAPIToken,
		model:    cfg.OpenAIAPIModel,
	}
}

// convertImageToBase64 通过调用 /resize 端点将图片 URL 转换为 base64 字符串
func (class *Client) convertImageToBase64(imageURL string, handle *processor.Handle) (string, error) {
	// 打印转换图像URL
	log.Printf("使用 Base64 编码图像: %s", imageURL)
	// 定义默认图片URL
	defaultUrl := handle.BaseURL + "/save/resources/placeholder/blank-02.png"
	// 创建 HTTP 客户端，设置 30 秒超时
	client := &http.Client{Timeout: 30 * time.Second}
	// 发起 GET 请求下载图片
	imgResp, err := client.Get(imageURL)
	// 检查图片下载是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("获取图片失败: %v", err)
	}
	// 确保下载响应体被关闭
	defer imgResp.Body.Close()
	// 检查下载响应状态码是否为 200 OK
	if imgResp.StatusCode != http.StatusOK {
		return defaultUrl, fmt.Errorf("获取图片失败，状态码: %d", imgResp.StatusCode)
	}
	// 读取下载的图片内容到字节切片
	imgBody, err := io.ReadAll(imgResp.Body)
	// 检查图片读取是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("读取图片失败: %v", err)
	}
	// 创建字节缓冲区用于构建 multipart 表单
	body := &bytes.Buffer{}
	// 创建 multipart 写入器
	writer := multipart.NewWriter(body)
	// 在 multipart 表单中创建名为 "image" 的文件字段，文件名为 "image.jpg"
	formFile, err := writer.CreateFormFile("image", "image.jpg")
	// 检查表单文件创建是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("创建表单文件失败: %v", err)
	}
	// 将图片字节数据写入表单文件字段
	if _, err = formFile.Write(imgBody); err != nil {
		return defaultUrl, fmt.Errorf("写入图片失败: %v", err)
	}
	// 关闭 multipart 写入器，写入结尾边界
	if err = writer.Close(); err != nil {
		return defaultUrl, fmt.Errorf("关闭writer失败: %v", err)
	}
	// 创建 POST 请求，目标地址为 handle.BaseURL + "/resize"
	req, err := http.NewRequest("POST", handle.BaseURL+"/resize", body)
	// 检查请求创建是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("创建请求失败: %v", err)
	}
	// 设置请求头 Content-Type 为 multipart 表单的类型
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// 发送 POST 请求
	resp, err := client.Do(req)
	// 检查请求发送是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("发送请求失败: %v", err)
	}
	// 确保响应体被关闭
	defer resp.Body.Close()
	// 检查响应状态码是否为 200 OK
	if resp.StatusCode != http.StatusOK {
		return defaultUrl, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}
	// 读取响应体内容
	respBody, err := io.ReadAll(resp.Body)
	// 检查响应读取是否成功
	if err != nil {
		return defaultUrl, fmt.Errorf("读取响应失败: %v", err)
	}
	// 定义变量保存解析后的 JSON 响应
	var resizeResp map[string]any
	// 将响应体解析为 JSON 字典
	if err := json.Unmarshal(respBody, &resizeResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	// 从字典中提取 "base64" 字段并断言为字符串
	if base64, ok := resizeResp["base64"].(string); ok {
		return base64, nil
	}
	// 如果字段不存在或类型不匹配，返回错误
	return defaultUrl, fmt.Errorf("响应中没有base64字段")
}

// processMessages 处理消息，将图片URL转换为base64
func (class *Client) processMessages(messages []processor.FusionMessage, handle *processor.Handle) []processor.MultimodalMessage {
	processedMessages := make([]processor.MultimodalMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if message, ok := msg.(processor.MultimodalMessage); ok {
			processedContent := make(processor.ProcessResult, len(message.Content))
			for j, contentItem := range message.Content {
				processedContent[j] = contentItem
				if itemMap, ok := contentItem.(processor.ImageMessage); ok {
					if itemMap.Type != "image_url" {
						continue
					}
					base64Str, err := class.convertImageToBase64(itemMap.ImageURL.URL, handle)
					if err != nil {
						log.Printf("转换图片URL失败: %v", err)
						continue
					}
					processedContent[j] = processor.ImageMessage{Type: "image_url", ImageURL: processor.ImageURL{URL: base64Str}}
				}
			}
			message.Content = processedContent
			processedMessages = append(processedMessages, message)
		}
	}
	return processedMessages
}

// sendRequest 发送HTTP请求并返回响应体
func (class *Client) sendRequest(processedMessages []processor.MultimodalMessage, ToolChoice string) ([]byte, error) {
	// 创建请求结构体
	request := Request{
		Model:      class.model,
		Messages:   make([]processor.FusionMessage, len(processedMessages)),
		Tools:      GetTools(ToolChoice),
		ToolChoice: ToolChoice,
	}
	// 复制处理后的消息到请求结构体
	for i, msg := range processedMessages {
		request.Messages[i] = msg
	}
	// 序列化请求结构体为JSON字符串
	buffer := &bytes.Buffer{}
	// 创建JSON编码器
	encoder := json.NewEncoder(buffer)
	// 禁用HTML转义
	encoder.SetEscapeHTML(false)
	// 编码请求结构体到JSON字符串
	if err := encoder.Encode(request); err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}
	// 移除JSON字符串首尾空格
	requestJSON := strings.TrimSpace(buffer.String())
	// 打印请求JSON字符串
	log.Printf("调用< %s >进行处理", class.agentURL)
	// 打印请求JSON字符串（调试用）
	if setup.DisplayDebugMessage {
		log.Printf("< OpenAI API 请求体 >:\n %s", requestJSON)
	}
	// 检查OpenAI API URL是否为空
	if class.agentURL == "" {
		return nil, fmt.Errorf("OpenAI API URL为空")
	}
	// 创建 HTTP POST 请求
	httpReq, err := http.NewRequest("POST", class.agentURL, strings.NewReader(requestJSON))
	// 检查请求创建是否成功
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	// 设置请求头 Content-Type 为 application/json
	httpReq.Header.Set("Content-Type", "application/json")
	// 设置请求头 Authorization 为 Bearer 令牌
	if class.token != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", class.token))
	}
	// 创建 HTTP 客户端，设置超时为 120 秒
	client := &http.Client{Timeout: 120 * time.Second}
	// 发送 HTTP 请求
	response, err := client.Do(httpReq)
	// 检查响应发送是否成功
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	// 关闭响应体读取流
	defer response.Body.Close()
	// 读取响应体内容
	responseBody, err := io.ReadAll(response.Body)
	// 检查响应读取是否成功
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}
	// 检查响应状态码是否为 200 OK
	if response.StatusCode != http.StatusOK {
		log.Printf("OpenAI API返回错误状态码: %d, 响应内容: %s", response.StatusCode, string(responseBody))
		return nil, fmt.Errorf("OpenAI API返回错误状态码: %d", response.StatusCode)
	}
	// 返回响应体内容
	return responseBody, nil
}

// processResponse 处理响应，提取助手回复内容
func (class *Client) processResponse(responseBody []byte, defaultReply string) (string, map[string]any, error) {
	var openAIResponse Response
	if err := json.Unmarshal(responseBody, &openAIResponse); err != nil {
		log.Printf("解析OpenAI API响应失败: %v, 响应内容: %s", err, string(responseBody))
		return defaultReply, nil, fmt.Errorf("解析响应失败: %v", err)
	}
	if len(openAIResponse.Choices) == 0 {
		return defaultReply, nil, nil
	}
	choice := openAIResponse.Choices[0]
	// 检查content字段是否存在
	if contentVal, ok := choice.Message["content"].(string); ok {
		// 剔除最开始出现的一对方括号及其内部的文本
		agentSpeech := regexp.MustCompile(`^\[.*?\]:`).ReplaceAllString(contentVal, "")
		// 去除首尾空格
		agentSpeech = strings.TrimSpace(agentSpeech)
		return agentSpeech, choice.Message, nil
	}
	// 如果content字段不存在，返回默认回复
	return defaultReply, choice.Message, nil
}

// handleToolCalls 处理工具调用
func (class *Client) handleToolCalls(toolCalls []any, messages []processor.FusionMessage, handle *processor.Handle) ([]processor.FusionMessage, string, error) {
	for _, toolCallItem := range toolCalls {
		if toolCallMap, ok := toolCallItem.(map[string]any); ok {
			if toolCallType, ok := toolCallMap["type"].(string); ok && toolCallType == "function" {
				if functionMap, ok := toolCallMap["function"].(map[string]any); ok {
					functionName := functionMap["name"].(string)
					arguments := functionMap["arguments"].(string)
					toolCallID := toolCallMap["id"].(string)
					// 打印工具调用信息
					log.Printf("执行工具: %s", functionName)
					// 创建工具调用对象
					toolCall := processor.ToolCall{
						Type: "function",
						ID:   toolCallID,
						Function: processor.ToolCallFunction{
							Name:      functionName,
							Arguments: arguments,
						},
					}
					// 执行工具调用
					result, err := ExecuteTool(toolCall, handle)
					// 检查是否有错误
					if err != nil {
						result = fmt.Sprintf("工具执行失败: %v", err)
					}
					// 创建工具响应消息
					toolResponse := processor.BaseMessage{
						Role:       "tool",
						Content:    result,
						ToolCallID: toolCallID,
						Name:       functionName,
					}
					// 将工具响应消息添加到消息列表中
					messages = append(messages, toolResponse)
				}
			}
		}
	}
	return messages, "none", nil
}

// CallAgent 调用Agent API
func (class *Client) CallAgent(messages []processor.FusionMessage, handle *processor.Handle, ToolChoice string) (string, error) {
	// 初始化助手回复内容
	agentSpeech := handle.Config.DefaultReply
	// 处理消息，将图片URL转换为base64
	processedMessages := class.processMessages(messages, handle)
	// 发送请求并获取响应
	responseBody, err := class.sendRequest(processedMessages, ToolChoice)
	// 检查是否有错误
	if err != nil {
		return agentSpeech, err
	}
	// 解析响应
	agentSpeech, message, err := class.processResponse(responseBody, handle.Config.DefaultReply)
	// 检查是否有错误
	if err != nil {
		return agentSpeech, err
	}
	// 检查是否有工具调用
	if toolCalls, ok := message["tool_calls"].([]any); ok && len(toolCalls) > 0 {
		// 执行工具调用
		messages, ToolChoice, err = class.handleToolCalls(toolCalls, messages, handle)
		// 检查是否有错误
		if err != nil {
			return agentSpeech, err
		}
		// 递归调用API，继续处理工具响应
		return class.CallAgent(messages, handle, ToolChoice)
	}
	// 返回助手回复内容
	return agentSpeech, nil
}
