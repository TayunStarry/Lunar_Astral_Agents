package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// embeddingRequest 对应 OpenAI 兼容 /v1/embeddings 请求体
type embeddingRequest struct {
	Model string   `json:"model"` // 模型名，固定 [system-embedding]
	Input []string `json:"input"` // 待嵌入文本列表
}

// embeddingData 对应响应 data 数组单项
type embeddingData struct {
	Embedding []float32 `json:"embedding"` // 嵌入向量
}

// embeddingResponse 对应 OpenAI 兼容 /v1/embeddings 响应体
type embeddingResponse struct {
	Data  []embeddingData `json:"data"` // 嵌入向量列表，与 input 等长
	Error *struct {
		Message string `json:"message"` // 错误描述
	} `json:"error,omitempty"` // 错误响应载荷
}

// embedTexts 批量调用 /v1/embeddings 获取嵌入向量
// model 参数指定嵌入模型名（通常为集合锁定的 Model 字段）
// 返回向量切片与输入文本切片等长且一一对应
func (d *MemoryDB) embedTexts(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if !d.memoryInitialized {
		return nil, fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	if d.embeddingBaseURL == "" {
		return nil, fmt.Errorf("嵌入服务 base_url 未配置")
	}
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: model,
		Input: texts,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化嵌入请求失败: %w", err)
	}

	// base_url 约定已含 /v1 前缀（与 chat completions 一致），仅追加 /embeddings
	apiURL := strings.TrimRight(d.embeddingBaseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建嵌入请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.embeddingAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.embeddingAPIKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求嵌入服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取嵌入响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp embeddingResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("嵌入服务错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("嵌入服务返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("嵌入响应数量不匹配: 期望 %d, 实际 %d", len(texts), len(embResp.Data))
	}

	result := make([][]float32, len(embResp.Data))
	for i, item := range embResp.Data {
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("嵌入响应第 %d 项向量为空", i)
		}
		result[i] = item.Embedding
	}
	return result, nil
}

// embedText 嵌入单条文本，返回对应向量
func (d *MemoryDB) embedText(ctx context.Context, model string, text string) ([]float32, error) {
	vecs, err := d.embedTexts(ctx, model, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}
