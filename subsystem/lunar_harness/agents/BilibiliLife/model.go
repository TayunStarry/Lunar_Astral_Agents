package BilibiliLife

// 生活链 · B 站真实浏览器版：模型调用（OpenAI 兼容 /chat/completions）。
//
// 与 goja 版不同，这里直接由 Go 发请求：文本用于评论/动态/回复，
// 图像序列帧用于 60 秒切片理解。模型地址与多模态模型沿用 lunar_config.json。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// modelUsage OpenAI 兼容响应中的 token 用量
type modelUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
}

type imageURL struct {
	URL string `json:"url"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

var (
	modelOnce  sync.Once
	modelName  string
	modelURL   string
	modelKey   string
	httpClient = &http.Client{Timeout: 5 * time.Minute}
)

// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End，
// 让 TUI「核心智能 实况」能看到生活链的模型调用，而不是只能干等。
var (
	modelObserverMu sync.Mutex
	modelBegin      func(role string)
	modelDelta      func(role, kind, text string)
	modelEnd        func(role string)
	// modelMeter 模型调用计量回调：生活链的调用同样计入「运行状态」的模型计量
	modelMeter func(prompt, completion int64, generation time.Duration, estimated bool)
)

func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string), meter func(prompt, completion int64, generation time.Duration, estimated bool)) {
	modelObserverMu.Lock()
	defer modelObserverMu.Unlock()
	modelBegin, modelDelta, modelEnd, modelMeter = begin, delta, end, meter
}

const lifeModelRole = "哔哩生活"

func observeBegin() {
	modelObserverMu.Lock()
	fn := modelBegin
	modelObserverMu.Unlock()
	if fn != nil {
		fn(lifeModelRole)
	}
}

func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	modelObserverMu.Lock()
	fn := modelDelta
	modelObserverMu.Unlock()
	if fn != nil {
		fn(lifeModelRole, kind, text)
	}
}

func observeEnd() {
	modelObserverMu.Lock()
	fn := modelEnd
	modelObserverMu.Unlock()
	if fn != nil {
		fn(lifeModelRole)
	}
}

// reportModelCall 把一次调用计入「运行状态」的模型计量：
// 优先用服务端回传的真实 usage，缺失时按请求体与输出文本长度估算（标记 estimated）。
func reportModelCall(requestBody []byte, completionText string, usage *modelUsage, elapsed time.Duration) {
	modelObserverMu.Lock()
	fn := modelMeter
	modelObserverMu.Unlock()
	if fn == nil {
		return
	}
	if usage != nil && (usage.Prompt > 0 || usage.Completion > 0) {
		fn(usage.Prompt, usage.Completion, elapsed, false)
		return
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	completion := int64(utf8.RuneCountInString(completionText))
	fn(prompt, completion, elapsed, true)
}

func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(config.AgentMultimodalModel)
		if modelName == "" {
			modelName = "system-multimodal"
		}
		modelURL = strings.TrimRight(strings.TrimSpace(config.AgentMultimodalURL), "/")
		if modelURL == "" {
			modelURL = config.LocalV1URL()
		}
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
		modelKey = strings.TrimSpace(config.AgentMultimodalKey)
	})
}

func chatCompletion(messages []chatMessage) (string, error) {
	observeBegin()
	defer observeEnd()
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   true,
		// 让服务端在流末回传 usage，模型计量才拿得到真实 token
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return "", fmt.Errorf("序列化模型请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		observeDelta("content", "（模型请求失败："+err.Error()+"）")
		return "", fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		observeDelta("content", fmt.Sprintf("（模型 HTTP %d）", resp.StatusCode))
		return "", fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 240))
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		text, reasoning, usage, err := readModelStream(resp.Body)
		if err != nil {
			return "", err
		}
		reportModelCall(body, text+reasoning, usage, time.Since(start))
		return text, nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取模型响应失败: %w", err)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *modelUsage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("模型返回空 choices")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	observeDelta("content", text)
	reportModelCall(body, text+out.Choices[0].Message.ReasoningContent, out.Usage, time.Since(start))
	return text, nil
}

// readModelStream 解析 OpenAI 兼容 SSE，边收边喂给「核心智能 实况」；同时取回思考文本与用量。
func readModelStream(body io.Reader) (string, string, *modelUsage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content strings.Builder
	var reasoning strings.Builder
	var usage *modelUsage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *modelUsage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", "", nil, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
				observeDelta("think", choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				observeDelta("content", choice.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", nil, fmt.Errorf("读取模型流失败: %w", err)
	}
	text := strings.TrimSpace(content.String())
	if text == "" {
		return "", "", nil, fmt.Errorf("模型返回空内容")
	}
	return text, reasoning.String(), usage, nil
}

func chatText(system, user string) (string, error) {
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

func chatWithImages(system, prompt string, images []string) (string, error) {
	parts := make([]contentPart, 0, len(images)+1)
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: image}})
	}
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	})
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
