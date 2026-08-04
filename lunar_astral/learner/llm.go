package learner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewLLMClient 创建 LLM 客户端
func NewLLMClient(cfg LearnerConfig) *LLMClient {
	return &LLMClient{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		httpClient:  &http.Client{Timeout: 180 * time.Second},
	}
}

// Chat 单次 LLM 调用
func (c *LLMClient) Chat(messages []LLMMessage, budget TokenBudget) (*LLMResponse, error) {
	// 裁剪消息到 token 预算内
	trimmed := c.trimMessagesToBudget(messages, budget)
	return c.callAPI(trimmed, budget)
}

// callAPI 执行一次 LLM API 调用
func (c *LLMClient) callAPI(messages []LLMMessage, budget TokenBudget) (*LLMResponse, error) {
	reqBody := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   budget.MaxOutput,
		Temperature: c.temperature,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	apiURL := c.baseURL + "/chat/completions"
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 LLM 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp chatCompletionError
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM 服务错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM 服务返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 返回空响应")
	}

	choice := chatResp.Choices[0]
	return &LLMResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
	}, nil
}

// estimateTokenCount 估算消息列表的 token 数
func (c *LLMClient) estimateTokenCount(messages []LLMMessage) int {
	totalChars := 0
	for _, msg := range messages {
		totalChars += len([]rune(msg.Content))
		for _, tc := range msg.ToolCalls {
			totalChars += len([]rune(tc.Function.Name))
			totalChars += len([]rune(tc.Function.Arguments))
		}
	}
	if totalChars == 0 {
		return 0
	}
	return int(float64(totalChars) / CharPerToken)
}

// trimMessagesToBudget 将消息列表裁剪到 token 预算内
// 保留策略：始终保留第一条消息（系统提示）和最后面的消息，中间按需截断
func (c *LLMClient) trimMessagesToBudget(messages []LLMMessage, budget TokenBudget) []LLMMessage {
	if len(messages) <= 2 {
		return messages
	}

	estimated := c.estimateTokenCount(messages)
	if estimated <= budget.MaxInput {
		return messages
	}

	// 保留第一条（系统提示）和最后面的消息
	first := messages[0]
	rest := messages[1:]

	for len(rest) > 1 {
		rest = rest[1:]
		candidate := append([]LLMMessage{first}, rest...)
		if c.estimateTokenCount(candidate) <= budget.MaxInput {
			return candidate
		}
	}

	// 如果只剩系统提示 + 最后一条仍然超限，截断最后一条内容
	last := rest[len(rest)-1]
	maxChars := int(float64(budget.MaxInput-200) * CharPerToken)
	runes := []rune(last.Content)
	if len(runes) > maxChars {
		last.Content = string(runes[:maxChars]) + "\n\n[上下文已按 token 预算截断]"
	}

	return []LLMMessage{first, last}
}

// ============================================================
// JSON 提取工具
// ============================================================

// extractJSON 从文本中提取 JSON 内容
// 处理 markdown 代码块包裹等情况
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// 去除 markdown 代码块包裹
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	// 尝试找到 JSON 数组或对象
	startIdx := -1
	endIdx := -1

	for i, ch := range text {
		if ch == '[' || ch == '{' {
			if startIdx == -1 {
				startIdx = i
			}
		}
		if ch == ']' || ch == '}' {
			endIdx = i
		}
	}

	if startIdx >= 0 && endIdx > startIdx {
		return text[startIdx : endIdx+1]
	}

	return text
}

// ============================================================
// 文本工具
// ============================================================

// truncateRunes 截断文本到指定字符数（rune 级别）
func truncateRunes(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "…"
}

// isGarbledText 检查文本是否包含乱码或异常字符
// 返回 true 表示文本异常
func isGarbledText(text string) bool {
	if text == "" {
		return true
	}

	runes := []rune(text)

	// 检查是否包含过多控制字符或替换字符（U+FFFD）
	garbledCount := 0
	for _, r := range runes {
		if r == '\uFFFD' || // Unicode 替换字符
			r == '\u0000' || // NULL
			(r < 32 && r != '\n' && r != '\r' && r != '\t') { // 其他控制字符
			garbledCount++
		}
	}

	// 如果乱码字符超过 5%，视为异常
	if float64(garbledCount)/float64(len(runes)) > 0.05 {
		return true
	}

	return false
}

// isValidReport 验证报告格式是否正确可用
// 检查最小长度和是否包含乱码
func isValidReport(report string) bool {
	runes := []rune(report)
	if len(runes) < MinReportLength {
		return false
	}
	if isGarbledText(report) {
		return false
	}
	return true
}