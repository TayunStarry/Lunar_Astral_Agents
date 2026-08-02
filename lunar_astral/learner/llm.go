package learner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"logger"
)

// LLMClient 独立的 LLM 客户端
// 直接调用 /v1/chat/completions，支持工具调用循环和 token 预算控制
type LLMClient struct {
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

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

// Chat 单次 LLM 调用（无工具）
func (c *LLMClient) Chat(messages []LLMMessage, budget TokenBudget) (*LLMResponse, error) {
	return c.callAPI(messages, nil, budget, "")
}

// ChatWithTools 单次 LLM 调用（带工具定义）
func (c *LLMClient) ChatWithTools(messages []LLMMessage, tools []ToolDefinition, budget TokenBudget) (*LLMResponse, error) {
	return c.callAPI(messages, tools, budget, "auto")
}

// ChatWithToolLoop 带工具调用循环的 LLM 调用
// 循环执行：调用 LLM → 解析 tool_calls → 执行工具 → 回写结果 → 再次调用
// 直到模型不再调用工具或达到最大轮次
// 返回最终 LLM 响应、完整消息历史（含工具调用记录）、错误
func (c *LLMClient) ChatWithToolLoop(
	messages []LLMMessage,
	tools []ToolDefinition,
	toolExecutor func(ToolCall) (string, error),
	budget TokenBudget,
	maxRounds int,
) (*LLMResponse, []LLMMessage, error) {
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolCallRounds
	}

	// 工作副本
	workingMessages := make([]LLMMessage, len(messages))
	copy(workingMessages, messages)

	var lastResponse *LLMResponse

	for i := 0; i < maxRounds; i++ {
		// 裁剪消息到 token 预算内
		trimmed := c.trimMessagesToBudget(workingMessages, budget)

		resp, err := c.callAPI(trimmed, tools, budget, "auto")
		if err != nil {
			return nil, workingMessages, fmt.Errorf("工具调用循环第 %d 轮 LLM 调用失败: %w", i+1, err)
		}

		lastResponse = resp

		// 如果模型没有调用工具，结束循环
		if len(resp.ToolCalls) == 0 {
			return resp, workingMessages, nil
		}

		// 构建助手消息（包含工具调用信息）
		assistantMsg := LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		workingMessages = append(workingMessages, assistantMsg)

		// 遍历执行所有工具调用
		for _, tc := range resp.ToolCalls {
			logger.Info("Learner", "执行工具: %s (id=%s)", tc.Function.Name, tc.ID)

			result, err := toolExecutor(tc)
			if err != nil {
				result = fmt.Sprintf("工具执行失败: %v", err)
				logger.Error("Learner", "工具 %s 执行失败: %v", tc.Function.Name, err)
			}

			// 将工具执行结果写入消息历史
			workingMessages = append(workingMessages, LLMMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	logger.Warn("Learner", "工具调用循环达到最大轮次 %d，强制终止", maxRounds)
	return lastResponse, workingMessages, nil
}

// callAPI 执行一次 LLM API 调用
func (c *LLMClient) callAPI(messages []LLMMessage, tools []ToolDefinition, budget TokenBudget, toolChoice string) (*LLMResponse, error) {
	reqBody := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   budget.MaxOutput,
		Temperature: c.temperature,
	}

	// 工具相关
	if len(tools) > 0 {
		reqBody.Tools = tools
		reqBody.ToolChoice = toolChoice
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
	result := &LLMResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
	}

	// 解析工具调用
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = choice.Message.ToolCalls
	}

	return result, nil
}

// estimateTokenCount 估算消息列表的 token 数
// 基于字符数 / CharPerToken 的粗略估算
func (c *LLMClient) estimateTokenCount(messages []LLMMessage) int {
	totalChars := 0
	for _, msg := range messages {
		totalChars += len([]rune(msg.Content))
		// 工具调用也计入
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
// 保留策略：始终保留第一条消息（系统提示）和最后 N 条消息，中间按需截断
func (c *LLMClient) trimMessagesToBudget(messages []LLMMessage, budget TokenBudget) []LLMMessage {
	if len(messages) <= 2 {
		return messages
	}

	estimated := c.estimateTokenCount(messages)
	if estimated <= budget.MaxInput {
		return messages
	}

	// 保留第一条（系统提示）和最后面的消息
	// 逐步丢弃中间的消息直到符合预算
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
	maxChars := int(float64(budget.MaxInput-200) * CharPerToken) // 留 200 token 给系统提示
	runes := []rune(last.Content)
	if len(runes) > maxChars {
		last.Content = string(runes[:maxChars]) + "\n\n[上下文已按 token 预算截断]"
	}

	return []LLMMessage{first, last}
}

// truncateText 截断文本到指定字符数
func truncateText(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "…"
}
