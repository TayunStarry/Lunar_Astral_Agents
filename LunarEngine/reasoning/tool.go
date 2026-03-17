package execute

import (
	config "Lunar-Astral-Agents/parameter"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetDynamicSystemPrompt 获取动态系统提示词 - 直接读取文件
func GetDynamicSystemPrompt() (string, error) {
	filePath := filepath.Join(*config.LocalDir, "resources/prompts/externalDialogue.md")
	body, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取系统提示词文件失败: %w", err)
	}
	promptContent := string(body)
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	promptContent = strings.ReplaceAll(promptContent, "{current-time}", currentTime)
	promptContent = strings.ReplaceAll(promptContent, "{current-address}", "最终档案馆-[神之梦]档案室")
	return promptContent, nil
}

// GetEmbeddingVector 获取嵌入向量
func GetEmbeddingVector(text string) ([]float64, error) {
	embeddingReq := map[string]interface{}{
		"input": text,
		"model": "system-embedding",
	}
	embeddingURL := fmt.Sprintf("https://localhost:%d/v1/embeddings", *config.BasicPort)
	jsonData, err := json.Marshal(embeddingReq)
	if err != nil {
		return nil, fmt.Errorf("序列化嵌入请求失败: %w", err)
	}
	resp, err := http.Post(embeddingURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("发送嵌入请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取嵌入向量失败，状态码: %d", resp.StatusCode)
	}
	// 解析嵌入响应
	var respond embeddingResp
	if err := json.NewDecoder(resp.Body).Decode(&respond); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}
	if len(respond.Data) == 0 {
		return nil, fmt.Errorf("嵌入响应中无数据")
	}
	return respond.Data[0].Embedding, nil
}

// QueryKnowledgeBase 查询知识库
func QueryKnowledgeBase(queryVector []float64) ([]Message, error) {
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

// GetKnowledgeMessages 获取知识消息
func GetKnowledgeMessages(latestContent string) ([]Message, error) {
	if latestContent == "" {
		return []Message{}, nil
	}
	// 获取嵌入向量
	queryVector, err := GetEmbeddingVector(latestContent)
	if err != nil {
		return []Message{}, fmt.Errorf("获取嵌入向量失败: %w", err)
	}
	// 限制向量长度为256
	if len(queryVector) > 256 {
		queryVector = queryVector[:256]
	}
	// 查询知识库
	knowledgeEntries, err := QueryKnowledgeBase(queryVector)
	if err != nil {
		return []Message{}, fmt.Errorf("查询知识库失败: %w", err)
	}
	// 构建知识消息
	var knowledgeMessages []Message
	for _, entry := range knowledgeEntries {
		if content, ok := entry.Content.(string); ok && content != "" {
			knowledgeMessages = append(knowledgeMessages, Message{Role: "user", Content: content})
		}
	}
	return knowledgeMessages, nil
}
