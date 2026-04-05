package model

import (
	"Lunar-Astral-Agents/parameter" // 导入配置包，用于获取本地目录
	"fmt"                           // 导入fmt包，用于格式化输出
	"os"                            // 导入os包，用于读取文件
	"path/filepath"                 // 导入filepath包，用于处理文件路径
	"strings"                       // 导入strings包，用于字符串操作
	"time"                          // 导入time包，用于处理时间
)

// GetSystemPrompt 获取系统提示词
func GetSystemPrompt() (string, error) {
	filePath := filepath.Join(*parameter.LocalDir, "resources/prompts/systemPrompt.md")
	body, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取系统提示词文件失败: %w", err)
	}
	promptContent := string(body)
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	promptContent = strings.ReplaceAll(promptContent, "{current-time}", currentTime)
	address := parameter.ServerAddress
	addressStr := address[0] + "-" + address[1]
	promptContent = strings.ReplaceAll(promptContent, "{current-address}", addressStr)
	return promptContent, nil
}

// ProcessSystemPrompt 处理系统提示词：如果客户端提供了系统提示词则使用，否则使用动态系统提示词
func ProcessSystemPrompt(messages []Message) (Message, []Message, error) {
	// 存储系统提示词消息
	var systemMessage Message
	// 存储非系统消息
	var nonSystemMessages []Message
	// 标记是否存在客户端提供的系统提示词
	hasClientSystemPrompt := false
	// 分离系统消息和非系统消息
	for _, msg := range messages {
		if msg.Role == "system" {
			// 检查系统消息内容是否为字符串类型
			if contentStr, ok := msg.Content.(string); ok {
				systemMessage = Message{Role: "system", Content: contentStr}
				hasClientSystemPrompt = true
			}
		} else {
			nonSystemMessages = append(nonSystemMessages, msg)
		}
	}
	// 如果没有客户端提供的系统提示词，使用动态系统提示词
	if !hasClientSystemPrompt {
		systemPrompt, err := GetSystemPrompt()
		if err != nil {
			return Message{}, nil, fmt.Errorf("获取系统提示词失败: %w", err)
		}
		systemMessage = Message{Role: "system", Content: systemPrompt}
	}
	return systemMessage, nonSystemMessages, nil
}
