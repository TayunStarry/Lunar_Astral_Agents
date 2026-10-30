package DialogueBridging

// model.go 规划模型调用：OpenAI 兼容 /chat/completions，流式并受思考监管约束。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// SetModelObserver 注入实况观察者与模型计量（app 启动时调用；nil 表示不接入）。
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string),
	meter func(prompt, completion, cached int64, generation time.Duration, estimated bool)) {
	observerMu.Lock()
	defer observerMu.Unlock()
	obsBegin, obsDelta, obsEnd, obsMeter = begin, delta, end, meter
}

func observeBegin() {
	observerMu.Lock()
	fn := obsBegin
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := obsDelta
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel, kind, text)
	}
}

func observeEnd() {
	observerMu.Lock()
	fn := obsEnd
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

func reportModelCall(requestBody []byte, completion string, usage *modelUsage, elapsed time.Duration) {
	observerMu.Lock()
	fn := obsMeter
	observerMu.Unlock()
	if fn == nil {
		return
	}
	if usage != nil && (usage.Prompt > 0 || usage.Completion > 0) {
		cached := int64(0)
		if usage.PromptTokensDetails != nil {
			cached = usage.PromptTokensDetails.CachedTokens
		}
		fn(usage.Prompt, usage.Completion, cached, elapsed, false)
		return
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	fn(prompt, int64(utf8.RuneCountInString(completion)), 0, elapsed, true)
}

// resolveModel 解析主多模态模型端点（只解析一次）
func resolveModel() {
	modelOnce.Do(func() {
		modelName = config.AgentMultimodalModel
		modelURL = strings.TrimRight(config.AgentMultimodalURL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
	})
}

// chatText 带思考监管的规划调用：命中重复时中断并重试。
func chatText(system, user string) (string, error) {
	for attempt := 0; ; attempt++ {
		text, err := chatOnce(system, user)
		if err == nil {
			return text, nil
		}
		if !errors.Is(err, loopguard.ErrLoop) || attempt >= loopMaxRetries {
			return "", err
		}
		logger.Warn("DialogueBridging", "规划模型陷入重复输出，已中断并重试（%d/%d）", attempt+1, loopMaxRetries)
	}
}

// chatOnce 单次规划调用（流式输出同步进实况）
func chatOnce(system, user string) (string, error) {
	observeBegin()
	defer observeEnd()
	resolveModel()

	body, err := json.Marshal(map[string]any{
		"model": modelName,
		"messages": []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return "", fmt.Errorf("序列化模型请求失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := modelHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 240))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		guard := loopguard.New(cancel, loopguard.DefaultOptions())
		guard.Start()
		text, reasoning, usage, readErr := readStream(resp.Body, guard)
		guard.Stop()
		if guard.Hit() {
			return "", loopguard.ErrLoop
		}
		if readErr != nil {
			return "", readErr
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
	if loopguard.Detected(text, loopguard.DefaultOptions()) {
		return "", loopguard.ErrLoop
	}
	observeDelta("content", text)
	reportModelCall(body, text+out.Choices[0].Message.ReasoningContent, out.Usage, time.Since(start))
	return text, nil
}

// readStream 解析 SSE：思考与正文分别累积并推送实况。
func readStream(body io.Reader, guard *loopguard.Guard) (string, string, *modelUsage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content, reasoning strings.Builder
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
				guard.Feed(loopguard.KindThink, choice.Delta.ReasoningContent)
				observeDelta("think", choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				guard.Feed(loopguard.KindContent, choice.Delta.Content)
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

// truncate 按字符截断并追加省略号
func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
