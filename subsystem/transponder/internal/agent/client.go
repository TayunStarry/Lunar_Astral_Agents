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

// CallAgent 调用Agent API
func (class *Client) CallAgent(messages []processor.FusionMessage, handle *processor.Handle, depth int) (string, error) {
	// 初始化助手回复内容
	agentSpeech := handle.Config.DefaultReply
	// 处理消息，将图片URL转换为base64
	processedMessages := make([]processor.MultimodalMessage, 0, len(messages))
	// 遍历消息，处理图片URL
	for _, msg := range messages {
		// 检查消息是否为nil
		if msg == nil {
			continue
		}
		// 安全类型断言
		if message, ok := msg.(processor.MultimodalMessage); ok {
			// 初始化消息内容数组
			processedContent := make(processor.ProcessResult, len(message.Content))
			// 遍历消息内容，处理图片URL
			for j, contentItem := range message.Content {
				processedContent[j] = contentItem
				if itemMap, ok := contentItem.(processor.ImageMessage); ok {
					// 检查是否为image_url类型
					if itemMap.Type != "image_url" {
						continue
					}
					// 转换图片URL为base64
					base64Str, err := class.convertImageToBase64(itemMap.ImageURL.URL, handle)
					if err != nil {
						log.Printf("转换图片URL失败: %v", err)
						continue
					}
					// 创建新的内容项，将image_url替换为base64
					processedContent[j] = processor.ImageMessage{Type: "image_url", ImageURL: processor.ImageURL{URL: base64Str}}
				}
			}
			// 更新消息的content
			message.Content = processedContent
			// 并入处理后的消息
			processedMessages = append(processedMessages, message)
		}
	}
	// 创建请求
	request := Request{
		Model:      class.model,
		Messages:   make([]processor.FusionMessage, len(processedMessages)),
		Tools:      GetTools(),
		ToolChoice: "auto",
	}
	// 将处理后的消息转换为FusionMessage
	for i, msg := range processedMessages {
		request.Messages[i] = msg
	}
	// 序列化请求
	buffer := &bytes.Buffer{}
	// 创建JSON编码器
	encoder := json.NewEncoder(buffer)
	// 禁用HTML转义
	encoder.SetEscapeHTML(false)
	// 编码请求
	if err := encoder.Encode(request); err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}
	// 移除首尾空格
	requestJSON := strings.TrimSpace(buffer.String())
	// 打印API地址
	log.Printf("调用< %s >进行处理", class.agentURL)
	// 判断是否显示调试信息
	if setup.DisplayDebugMessage {
		log.Printf("< OpenAI API 请求体 >:\n %s", requestJSON)
	}
	// 检查API URL是否为空
	if class.agentURL == "" {
		return agentSpeech, fmt.Errorf("OpenAI API URL为空")
	}
	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", class.agentURL, strings.NewReader(requestJSON))
	// 检查请求是否成功
	if err != nil {
		return agentSpeech, fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	// 设置Authorization头
	if class.token != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", class.token))
	}
	// 发送请求 并 设置超时时间为120秒
	client := &http.Client{Timeout: 120 * time.Second}
	// 发送请求
	response, err := client.Do(httpReq)
	// 检查请求是否成功
	if err != nil {
		return agentSpeech, fmt.Errorf("发送请求失败: %v", err)
	}
	// 关闭响应体
	defer response.Body.Close()
	// 读取响应
	responseBody, err := io.ReadAll(response.Body)
	// 检查响应状态码
	if err != nil {
		return agentSpeech, fmt.Errorf("读取响应失败: %v", err)
	}
	// 检查响应状态码
	if response.StatusCode != http.StatusOK {
		log.Printf("OpenAI API返回错误状态码: %d, 响应内容: %s", response.StatusCode, string(responseBody))
		return agentSpeech, fmt.Errorf("OpenAI API返回错误状态码: %d", response.StatusCode)
	}
	// 解析响应
	var openAIResponse Response
	// 解析响应
	if err := json.Unmarshal(responseBody, &openAIResponse); err != nil {
		log.Printf("解析OpenAI API响应失败: %v, 响应内容: %s", err, string(responseBody))
		return agentSpeech, fmt.Errorf("解析响应失败: %v", err)
	}
	// 检查是否有助手回复
	if len(openAIResponse.Choices) == 0 {
		return agentSpeech, nil
	}
	// 提取助手回复内容
	choice := openAIResponse.Choices[0]
	// 检查是否有工具调用
	if toolCalls, ok := choice.Message["tool_calls"].([]any); ok && len(toolCalls) > 0 {
		// 执行工具调用
		for _, toolCallItem := range toolCalls {
			if toolCallMap, ok := toolCallItem.(map[string]any); ok {
				if toolCallType, ok := toolCallMap["type"].(string); ok && toolCallType == "function" {
					if functionMap, ok := toolCallMap["function"].(map[string]any); ok {
						functionName := functionMap["name"].(string)
						arguments := functionMap["arguments"].(string)
						toolCallID := toolCallMap["id"].(string)

						log.Printf("执行工具: %s, 深度: %d", functionName, depth)

						// 创建ToolCall结构
						toolCall := processor.ToolCall{
							Type: "function",
							ID:   toolCallID,
							Function: processor.ToolCallFunction{
								Name:      functionName,
								Arguments: arguments,
							},
						}

						// 执行工具
						result, err := ExecuteTool(toolCall, handle)
						if err != nil {
							result = fmt.Sprintf("工具执行失败: %v", err)
						}

						// 检查工具调用ID是否为空
						if toolCallID == "" || depth >= 2 {
							return agentSpeech, nil
						}

						// 创建工具响应消息
						toolResponse := processor.BaseMessage{
							Role:       "tool",
							Content:    result,
							ToolCallID: toolCallID,
							Name:       functionName,
						}

						// 将工具响应添加到消息列表
						messages = append(messages, toolResponse)
					}
				}
			}
		}
		// 递归调用API，继续处理工具响应
		return class.CallAgent(messages, handle, depth+1)
	}
	// 检查content字段是否存在
	if contentVal, ok := choice.Message["content"].(string); ok {
		agentSpeech = contentVal
	}
	// 剔除最开始出现的一对方括号及其内部的文本
	agentSpeech = regexp.MustCompile(`^\[.*?\]:`).ReplaceAllString(agentSpeech, "")
	// 去除首尾空格
	agentSpeech = strings.TrimSpace(agentSpeech)
	// 返回助手回复内容
	return agentSpeech, nil
}
