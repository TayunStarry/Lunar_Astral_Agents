package agent

import (
	"bytes"                          // 用于处理字节缓冲区
	"encoding/json"                  // 用于JSON编码和解码
	"fmt"                            // 格式化输出
	"io"                             // 用于IO操作
	"log"                            // 日志记录
	"net/http"                       // HTTP客户端
	"regexp"                         // 用于正则表达式操作
	"strings"                        // 字符串操作
	"time"                           // 时间操作
	"transponder/internal/setup"    // 配置文件
	"transponder/internal/processor" // 核心处理器
)

// NewClient 创建新的OpenAI客户端
func NewClient(cfg *setup.Config) *Client {
	return &Client{
		agentURL: cfg.OpenAIAPIUrl,
		token:    cfg.OpenAIAPIToken,
		model:    cfg.OpenAIAPIModel,
	}
}

// CallAgent 调用Agent API
func (class *Client) CallAgent(messages []Message, processor *processor.Handle, depth int) (string, error) {
	// 创建请求
	request := Request{
		Model:      class.model,
		Messages:   messages,
		Tools:      GetTools(),
		ToolChoice: "auto",
	}
	/*
		jsonStr, err := json.Marshal(request)
		if err != nil {
			return "", fmt.Errorf("序列化请求失败: %v", err)
		}
		log.Printf("<请求> %s", jsonStr)
	*/
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
		log.Printf("%s", requestJSON)
	}
	// 检查API URL是否为空
	if class.agentURL == "" {
		return processor.Config.DefaultReply, fmt.Errorf("OpenAI API URL为空")
	}
	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", class.agentURL, strings.NewReader(requestJSON))
	// 检查请求是否成功
	if err != nil {
		return processor.Config.DefaultReply, fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	// 设置Authorization头
	if class.token != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", class.token))
	}
	// 发送请求 并 设置超时时间为120秒
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	// 发送请求
	response, err := client.Do(httpReq)
	// 检查请求是否成功
	if err != nil {
		return processor.Config.DefaultReply, fmt.Errorf("发送请求失败: %v", err)
	}
	defer response.Body.Close()
	// 读取响应
	responseBody, err := io.ReadAll(response.Body)
	// 检查响应状态码
	if err != nil {
		return processor.Config.DefaultReply, fmt.Errorf("读取响应失败: %v", err)
	}
	// 检查响应状态码
	if response.StatusCode != http.StatusOK {
		log.Printf("OpenAI API返回错误状态码: %d, 响应内容: %s", response.StatusCode, string(responseBody))
		return processor.Config.DefaultReply, fmt.Errorf("OpenAI API返回错误状态码: %d", response.StatusCode)
	}
	// 解析响应
	var openAIResponse Response
	// 解析响应
	if err := json.Unmarshal(responseBody, &openAIResponse); err != nil {
		log.Printf("解析OpenAI API响应失败: %v, 响应内容: %s", err, string(responseBody))
		return processor.Config.DefaultReply, fmt.Errorf("解析响应失败: %v", err)
	}
	// 检查是否有助手回复
	if len(openAIResponse.Choices) == 0 {
		return processor.Config.DefaultReply, nil
	}
	// 提取助手回复内容
	message := openAIResponse.Choices[0].Message
	// 检查是否有工具调用
	if len(message.ToolCalls) > 0 {
		// 执行工具调用
		for _, toolCall := range message.ToolCalls {
			log.Printf("执行工具: %s, 深度: %d", toolCall.Function.Name, depth)
			// 执行工具
			result, err := ExecuteTool(toolCall, processor)
			if err != nil {
				result = fmt.Sprintf("工具执行失败: %v", err)
			}
			// 检查工具调用ID是否为空
			if toolCall.ID == "" || depth >= 2 {
				return processor.Config.DefaultReply, nil
			}
			// 创建工具响应消息
			toolResponse := Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			}
			// 将工具响应添加到消息列表
			messages = append(messages, toolResponse)
		}
		// 递归调用API，继续处理工具响应
		return class.CallAgent(messages, processor, depth+1)
	}
	// 获取助手回复内容
	textResponse := message.Content.(string)
	//log.Printf("<响应> %s", string(textResponse))
	// 剔除最开始出现的一对方括号及其内部的文本
	textResponse = regexp.MustCompile(`^\[.*?\]:`).ReplaceAllString(textResponse, "")
	// 去除首尾空格
	textResponse = strings.TrimSpace(textResponse)
	// 返回助手回复内容
	return textResponse + " ", nil
}
