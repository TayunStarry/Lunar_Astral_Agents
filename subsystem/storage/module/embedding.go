package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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

// =============================================================================
// v2 LLM 标签生成 — 调用 /v1/chat/completions 自动生成中文标签
// =============================================================================

// chatRequest OpenAI 兼容 /v1/chat/completions 请求体
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []chatContentPart
}

// chatContentPart 多模态消息内容部分
type chatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片 URL 引用
type imageURL struct {
	URL string `json:"url"`
}

// chatResponse OpenAI 兼容 /v1/chat/completions 响应体
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// chatChoice 聊天响应选择
type chatChoice struct {
	Message chatMessageResp `json:"message"`
}

// chatMessageResp 聊天响应消息
type chatMessageResp struct {
	Content string `json:"content"`
}

// generateTags 调用 LLM 为内容生成中文标签，返回标签数组
// isImage 为 true 时使用多模态 vision 格式请求
// 最多重试 MaxTagRetries 次，全部失败则返回错误
func (d *MemoryDB) generateTags(ctx context.Context, content string, isImage bool) ([]string, error) {
	if d.llmBaseURL == "" {
		return nil, fmt.Errorf("LLM 服务 base_url 未配置")
	}

	var lastErr error
	for attempt := 0; attempt < MaxTagRetries; attempt++ {
		tags, err := d.generateTagsOnce(ctx, content, isImage)
		if err == nil && len(tags) > 0 {
			return tags, nil
		}
		lastErr = err
		if attempt < MaxTagRetries-1 {
			// 短暂等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil, fmt.Errorf("标签生成失败（已重试 %d 次）: %w", MaxTagRetries, lastErr)
}

// generateTagsOnce 单次 LLM 标签生成尝试
func (d *MemoryDB) generateTagsOnce(ctx context.Context, content string, isImage bool) ([]string, error) {
	apiURL := strings.TrimRight(d.llmBaseURL, "/") + "/chat/completions"

	var messages []chatMessage

	if isImage {
			// 多模态图片标签生成（v3 增强：OCR 文字提取 + 人物特征分析）
			systemMsg := chatMessage{
				Role: "system",
				Content: "你是一个视觉内容标签生成助手。请仔细观察图片，按以下规则生成标签，严格以JSON数组格式返回，不要包含任何其他内容。\n\n" +
					"标签生成规则：\n" +
					"1. 描述图片的整体内容主题和风格特点\n" +
					"2. 提取画面中的文字信息（OCR）：如有可见文字，生成对应标签\n" +
					"3. 若图片包含人物，额外提取以下特征：\n" +
					"   - 面部表情（如：微笑、严肃、惊讶、悲伤、愤怒）\n" +
					"   - 肢体动作（如：站立、挥手、奔跑、坐着、跳舞）\n" +
					"   - 头发颜色（如：黑色头发、金色头发、棕色头发、红色头发）\n" +
					"   - 服饰风格与颜色（如：白色连衣裙、黑色西装、休闲T恤、校服）\n" +
					"4. 描述画面的色彩倾向和情感氛围\n" +
					"用中文输出，标签数量控制在5-15个。\n\n" +
					"示例输出：[\"自然风景\",\"日落\",\"暖色调\",\"海边\",\"宁静\",\"白色连衣裙\",\"微笑\",\"黑色长发\",\"站立\",\"夕阳余晖\"]",
			}
		userMsg := chatMessage{
			Role: "user",
			Content: []chatContentPart{
				{Type: "text", Text: "请为这张图片生成标签，以JSON数组格式返回"},
				{Type: "image_url", ImageURL: &imageURL{URL: content}},
			},
		}
		messages = []chatMessage{systemMsg, userMsg}
	} else {
		// 文本标签生成
		systemMsg := chatMessage{
			Role: "system",
			Content: "你是一个内容标签生成助手。请为以下文本生成标签，描述其核心主题和关键信息。用中文输出，严格以JSON数组格式返回，不要包含任何其他内容。\n\n" +
				"示例输出：[\"人工智能\",\"机器学习\",\"深度学习\",\"神经网络\"]",
		}
		userMsg := chatMessage{
			Role:    "user",
			Content: "请为以下文本生成标签，以JSON数组格式返回：\n\n" + content,
		}
		messages = []chatMessage{systemMsg, userMsg}
	}

	reqBody := chatRequest{
		Model:       d.multimodalModel,
		Messages:    messages,
		MaxTokens:   200,
		Temperature: 0.3,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 LLM 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 LLM 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.llmAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.llmAPIKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 LLM 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp chatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM 错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM 返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 响应无 choices")
	}

	rawContent := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if rawContent == "" {
		return nil, fmt.Errorf("LLM 返回空内容")
	}

	// 解析 JSON 数组：尝试从响应中提取 ["标签1","标签2",...]
	tags, err := parseTagsJSON(rawContent)
	if err != nil {
		return nil, fmt.Errorf("解析标签失败: %w (原始响应: %s)", err, truncateForLog(rawContent, 100))
	}

	return tags, nil
}

// parseTagsJSON 从 LLM 响应文本中提取 JSON 字符串数组
// 支持纯 JSON 数组、markdown 代码块包裹、以及带前后文字的混合格式
func parseTagsJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)

	// 去除 markdown 代码块标记
	if strings.HasPrefix(raw, "```") {
		// 找到第一个换行后的内容
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		// 去除结尾的 ```
		if strings.HasSuffix(raw, "```") {
			raw = raw[:len(raw)-3]
		}
		raw = strings.TrimSpace(raw)
	}

	// 尝试找到 JSON 数组 [...]
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("未找到 JSON 数组")
	}

	jsonStr := raw[start : end+1]

	var tags []string
	if err := json.Unmarshal([]byte(jsonStr), &tags); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	// 过滤空标签
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("解析到的标签数组为空")
	}

	return result, nil
}

// truncateForLog 截断字符串用于日志输出
func truncateForLog(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
