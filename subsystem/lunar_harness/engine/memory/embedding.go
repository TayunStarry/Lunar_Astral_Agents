package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// embedTexts 批量调用 /v1/embeddings 获取嵌入向量（document 语义：裸文本，不加前缀）
// model 参数指定嵌入模型名（通常为集合锁定的 Model 字段）
// 返回向量切片与输入文本切片等长且一一对应
// 注：Qwen3-Embedding 为指令感知模型，仅查询侧需要 Instruct/Query 前缀，见 embedQuery
func (d *Store) embedTexts(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if !d.initialized {
		return nil, fmt.Errorf("记忆库未初始化, 请先调用 EnsureReady")
	}
	embeddingURL := config.AgentEmbeddingURL
	if embeddingURL == "" {
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
	apiURL := strings.TrimRight(embeddingURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建嵌入请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if config.AgentEmbeddingKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.AgentEmbeddingKey)
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

// embedText 嵌入单条文本，返回对应向量（document 语义：裸文本，不加前缀）
func (d *Store) embedText(ctx context.Context, model string, text string) ([]float32, error) {
	vecs, err := d.embedTexts(ctx, model, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// queryInstruction Qwen3-Embedding 检索指令（官方推荐的英文任务描述）
// 与文档侧裸文本嵌入配合，官方评估显示使用指令可获得 1%~5% 性能提升
const queryInstruction = "Given a web search query, retrieve relevant passages that answer the query"

// embedQuery 嵌入查询文本（query 语义：带 Qwen3-Embedding 指令感知前缀）
// 官方规范：Query 侧为 "Instruct: {任务描述}\nQuery:{查询}"，Document 侧保持裸文本
// 文档向量已以裸文本嵌入，查询向量加前缀后新旧文档向量空间保持一致，无需重建
func (d *Store) embedQuery(ctx context.Context, model string, text string) ([]float32, error) {
	return d.embedText(ctx, model, "Instruct: "+queryInstruction+"\nQuery:"+text)
}

// ==== 记忆库之外的嵌入入口 ====
//
// 记忆库是嵌入配置（agent.embedding_*）的持有者，但嵌入本身并不等于“写记忆”。
// 动作匹配这类进程内索引需要同一套向量空间，又不该把数据写进记忆库，因此
// 这里提供只读的嵌入入口：调用方自己持有索引与生命周期。
//
// embeddingOnlyClient 供这类调用复用的 HTTP 客户端（http.Client 并发安全）。
var embeddingOnlyClient = &http.Client{Timeout: 120 * time.Second}

// EmbedDocuments 用当前嵌入模型嵌入一组文档文本（document 语义：裸文本）。
//
// 与记忆库内容向量保持同一向量空间；不读写任何集合/文档。
func EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	db := &Store{initialized: true, httpClient: embeddingOnlyClient}
	return db.embedTexts(ctx, currentEmbeddingModel(), texts)
}

// EmbedQuery 用当前嵌入模型嵌入查询文本（query 语义：带检索指令前缀）。
func EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	db := &Store{initialized: true, httpClient: embeddingOnlyClient}
	return db.embedQuery(ctx, currentEmbeddingModel(), text)
}
