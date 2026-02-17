package message

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"strings"
	status "transponder/internal/config"
)

// EmbeddingResponse 嵌入响应结构
type EmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// KnowledgeMessage 知识库消息结构
type KnowledgeMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	ImageUrl string `json:"imageUrl"`
	UUID     string `json:"uuid"`
}

// generateEmbedding 生成文本嵌入向量
func (class *Processor) generateEmbedding(text string) ([]float64, error) {
	// 构建请求体
	requestBody := map[string]interface{}{
		"model":          "system-embedding",
		"input":          []string{text},
		"task_type":      "search_document",
		"dimensionality": 256,
	}

	// 编码请求体为JSON
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("编码请求体失败: %v", err)
	}

	// 构建完整的请求URL
	requestURL := class.baseURL + "/v1/embeddings"
	log.Printf("调用< API : %s >进行处理", requestURL)

	// 创建请求
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查HTTP响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Printf("嵌入API返回非200状态码: %d", resp.StatusCode)
		log.Printf("响应内容: %s", string(respBody))
		return nil, fmt.Errorf("嵌入API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 检查响应体是否为空
	if len(respBody) == 0 {
		return nil, fmt.Errorf("嵌入API返回空响应")
	}

	// 检查响应体是否为有效的JSON
	var jsonTest interface{}
	if err := json.Unmarshal(respBody, &jsonTest); err != nil {
		log.Printf("嵌入API返回非JSON响应: %s", string(respBody))
		return nil, fmt.Errorf("嵌入API返回非JSON响应: %s, 错误: %v", string(respBody), err)
	}

	// 解析响应
	var embeddingResp EmbeddingResponse
	if err := json.Unmarshal(respBody, &embeddingResp); err != nil {
		log.Printf("嵌入API响应解析失败，响应内容: %s", string(respBody))
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	// 检查响应数据
	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("响应中没有嵌入数据")
	}

	// 返回嵌入向量的前256个元素
	embedding := embeddingResp.Data[0].Embedding
	if len(embedding) > 256 {
		embedding = embedding[:256]
	}
	return embedding, nil
}

// queryKnowledgeBase 查询知识库获取相关条目
func (class *Processor) queryKnowledgeBase(queryVector []float64) (*KnowledgeMessage, error) {
	// 构建请求体
	requestBody := map[string]interface{}{
		"filePath":    "knowledge/meme_model.json",
		"queryVector": queryVector,
		"topK":        5,
	}

	// 编码请求体为JSON
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("编码请求体失败: %v", err)
	}

	// 构建完整的请求URL
	requestURL := class.baseURL + "/knowledge/query"
	log.Printf("发送知识库查询请求到: %s", requestURL)

	// 创建请求
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查HTTP响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Printf("知识库API返回非200状态码: %d", resp.StatusCode)
		log.Printf("响应内容: %s", string(respBody))
		return nil, fmt.Errorf("知识库API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 检查响应体是否为空
	if len(respBody) == 0 {
		return nil, fmt.Errorf("知识库API返回空响应")
	}

	// 检查响应体是否为有效的JSON
	var jsonTest interface{}
	if err := json.Unmarshal(respBody, &jsonTest); err != nil {
		log.Printf("知识库API返回非JSON响应: %s", string(respBody))
		return nil, fmt.Errorf("知识库API返回非JSON响应: %s, 错误: %v", string(respBody), err)
	}

	// 解析响应
	var knowledgeMessages []KnowledgeMessage
	if err := json.Unmarshal(respBody, &knowledgeMessages); err != nil {
		log.Printf("知识库API响应解析失败，响应内容: %s", string(respBody))
		return nil, fmt.Errorf("解析响应失败: %v, 响应: %s", err, string(respBody))
	}

	// 查找第一个有图片的消息
	for _, msg := range knowledgeMessages {
		if msg.ImageUrl != "" {
			log.Printf("找到带图片的知识库条目，图片URL: %s", msg.ImageUrl)
			return &msg, nil
		}
	}

	log.Printf("知识库中没有找到带图片的相关条目")
	return nil, fmt.Errorf("知识库中没有找到带图片的相关条目")
}

// SaveToKnowledgeBase 保存内容到知识库
func (class *Processor) SaveToKnowledgeBase(content, filePath string) (string, error) {
	// 生成UUID
	uuidStr := uuid.New().String()
	embedVector, err := class.generateEmbedding(content)
	if err != nil {
		return "生成嵌入向量失败", err
	}
	// 构建请求体
	requestBody := map[string]interface{}{
		"filePath": filePath,
		"message": map[string]interface{}{
			"role":        "assistant",
			"content":     content,
			"isPrompt":    false,
			"noRender":    false,
			"imageUrl":    "",
			"deletable":   true,
			"uuid":        uuidStr,
			"embedVector": embedVector,
		},
	}

	// 编码请求体为JSON
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("编码请求体失败: %v", err)
	}

	// 发送POST请求到knowledge/write接口
	req, err := http.NewRequest("POST", class.baseURL+"/knowledge/write", strings.NewReader(string(body)))
	if err != nil {
		return "创建请求失败", err
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "发送请求失败", err
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "读取响应失败", err
	}

	// 检查HTTP响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Printf("知识库API返回非200状态码: %d", resp.StatusCode)
		log.Printf("响应内容: %s", string(respBody))
		return "知识库API返回错误状态码", nil
	}

	// 发送flush请求，确保内容写入文件
	flushBody := map[string]interface{}{
		"filePath": filePath,
	}
	flushJSON, err := json.Marshal(flushBody)
	if err != nil {
		return "编码flush请求体失败", err
	}

	flushReq, err := http.NewRequest("POST", class.baseURL+"/knowledge/flush", strings.NewReader(string(flushJSON)))
	if err != nil {
		return "创建flush请求失败", err
	}

	flushReq.Header.Set("Content-Type", "application/json")
	flushResp, err := client.Do(flushReq)
	if err != nil {
		return "发送flush请求失败", err
	}
	defer flushResp.Body.Close()

	if flushResp.StatusCode != http.StatusOK {
		flushRespBody, _ := io.ReadAll(flushResp.Body)
		log.Printf("知识库flush API返回非200状态码: %d", flushResp.StatusCode)
		log.Printf("响应内容: %s", string(flushRespBody))
	}

	return "内容已成功保存到知识库", nil
}

// SendGroupImageMsg 发送群图片消息
func (class *Processor) SendGroupImageMsg(groupID int64, imageUrl string) error {
	fullImageUrl := class.baseURL + imageUrl
	log.Printf("发送图片URL: %s", fullImageUrl)

	// 创建图片消息项
	message := []status.MsgItem{
		{
			Type: "image",
			Data: map[string]string{
				"url":  fullImageUrl,
				"file": fullImageUrl,
			},
		},
	}

	// 创建请求参数
	params := SendGroupMsgParams{
		GroupID: groupID,
		Message: message,
	}

	// 发送消息
	_, err := class.wsClient.SendMessage("send_group_msg", params)
	if err != nil {
		return fmt.Errorf("发送群图片消息失败: %v", err)
	}

	log.Printf("已向< QQ群 %d >发送图片消息", groupID)
	return nil
}
