package llama

// complete.go 原生补全调用：直接读取 llama.cpp 生成 token 的 id。
//
// 为什么不用 OpenAI 兼容的 /v1/chat/completions：
// MIDI-LLM 的输出全是词表扩展的 special token（<|midi_token_N|>），
// 文本层会把它们过滤成空字符串，拿不到乐谱内容。
// /completion 返回 completion_probabilities[].id，才是 AMT 词表的真实 token。

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

// midiTokenBase GGUF 中 MIDI token 的起始 id（Llama 3.2 基础词表大小）
const midiTokenBase = 128256

// completionRequest 原生补全请求体。
type completionRequest struct {
	// Model 路由服务器上的模型 ID
	Model string `json:"model"`
	// Prompt 完整提示（末尾需带 AMT 自回归起始 token）
	Prompt string `json:"prompt"`
	// NPredict 最大生成 token 数
	NPredict int `json:"n_predict"`
	// Temperature 采样温度
	Temperature float64 `json:"temperature"`
	// TopP 核采样
	TopP float64 `json:"top_p"`
	// TopK 候选数（0 表示不限制）
	TopK int `json:"top_k"`
	// NProbs 必须为 1，否则响应不带 token id
	NProbs int `json:"n_probs"`
	// Stream 非流式
	Stream bool `json:"stream"`
	// CachePrompt 不复用提示缓存
	CachePrompt bool `json:"cache_prompt"`
}

// completionResponse 原生补全响应体（只取需要的字段）。
type completionResponse struct {
	// CompletionProbabilities 逐 token 的生成结果（含词表 id）
	CompletionProbabilities []struct {
		ID int `json:"id"`
	} `json:"completion_probabilities"`
	// TokensPredicted 实际生成 token 数
	TokensPredicted int `json:"tokens_predicted"`
	// Error 错误信息
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CompletionTokens 调用路由服务器的 /completion，返回 AMT 词表 id 序列。
// MIDI-LLM 推荐 temperature 1.0 / top_p 0.98 / top_k 0。
func CompletionTokens(ctx context.Context, modelID, prompt string, maxTokens int) ([]int, error) {
	if strings.TrimSpace(modelID) == "" {
		return nil, fmt.Errorf("模型 ID 不能为空")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("提示不能为空")
	}
	if maxTokens <= 0 {
		maxTokens = 2046
	}
	payload := completionRequest{
		Model:       modelID,
		Prompt:      prompt,
		NPredict:    maxTokens,
		Temperature: 1.0,
		TopP:        0.98,
		TopK:        0,
		NProbs:      1,
		Stream:      false,
		CachePrompt: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化补全请求失败: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/completion", config.ModelPort)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建补全请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("补全请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("补全接口返回状态码 %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result completionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析补全响应失败: %w", err)
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return nil, fmt.Errorf("补全失败: %s", result.Error.Message)
	}
	ids := make([]int, 0, len(result.CompletionProbabilities))
	for _, item := range result.CompletionProbabilities {
		if item.ID >= midiTokenBase {
			ids = append(ids, item.ID-midiTokenBase)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("补全未返回 MIDI token")
	}
	return ids, nil
}
