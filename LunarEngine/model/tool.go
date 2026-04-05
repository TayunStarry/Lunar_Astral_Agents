package model

import (
	"Lunar-Astral-Agents/parameter" // 导入配置包，用于获取本地目录
	"bytes"                         // 导入bytes包，用于处理字节流
	"encoding/json"                 // 导入json包，用于解析JSON响应
	"fmt"                           // 导入fmt包，用于格式化输出
	"net/http"                      // 导入net/http包，用于发送HTTP请求
)

// GetEmbeddingVector 获取嵌入向量
func GetEmbeddingVector(text string) ([]float64, error) {
	embeddingReq := map[string]any{
		"input": text,
		"model": "system-embedding",
	}
	embeddingURL := fmt.Sprintf("http://localhost:%d/v1/embeddings", *parameter.BasicPort)
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
	knowledgeURL := fmt.Sprintf("http://localhost:%d/knowledge/query", *parameter.BasicPort)
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

// GetModelPort 根据模型名称获取对应端口（加读锁）
func GetModelPort(modelName string) (int, bool) {
	// 加读锁，防止并发修改模型端口映射时出现数据竞争
	parameter.ModelMapMutex.RLock()
	// 函数结束时解锁，确保锁一定会被释放
	defer parameter.ModelMapMutex.RUnlock()
	// 从模型端口映射中查找指定模型的端口号
	port, exists := parameter.ModelPortMap[modelName]
	// 返回端口号和是否存在的标志
	return port, exists
}
