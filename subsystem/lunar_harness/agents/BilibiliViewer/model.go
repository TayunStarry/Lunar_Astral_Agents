package BilibiliViewer

// 生活链 · B 站真实浏览器版：模型调用（OpenAI 兼容 /chat/completions）。
//
// 与 goja 版不同，这里直接由 Go 发请求：文本用于评论/动态/回复，
// 图像序列帧用于 60 秒切片理解。模型地址与多模态模型沿用 lunar_config.json。

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
	"sync"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// modelUsage OpenAI 兼容响应中的 token 用量
type modelUsage struct {
	Prompt              int64               `json:"prompt_tokens"`
	Completion          int64               `json:"completion_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（llama.cpp 回传的提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
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
	modelMeter func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)

func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string), meter func(prompt, completion, cached int64, generation time.Duration, estimated bool)) {
	modelObserverMu.Lock()
	defer modelObserverMu.Unlock()
	modelBegin, modelDelta, modelEnd, modelMeter = begin, delta, end, meter
}

const lifeModelRole = "哔哩观影"

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

// observeContent 把正文送入实况：display 为空时原样推送，否则先按它规整。
func observeContent(text string, display func(string) string) {
	if display != nil {
		text = display(text)
	}
	observeDelta("content", text)
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
		cached := int64(0)
		if usage.PromptTokensDetails != nil {
			cached = usage.PromptTokensDetails.CachedTokens
		}
		fn(usage.Prompt, usage.Completion, cached, elapsed, false)
		return
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	completion := int64(utf8.RuneCountInString(completionText))
	fn(prompt, completion, 0, elapsed, true)
}

func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(config.AgentMultimodalModel)
		if modelName == "" {
			modelName = "primary_reasoning"
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

// chatCompletion 带思考监管的模型调用：输出卡循环时中断并重新生成，
// 最多重试 lifeLoopMaxRetries 次；仍失败则返回错误，由调用方跳过本次流程。
func chatCompletion(messages []chatMessage) (string, error) {
	return chatCompletionWith(messages, nil, loopguard.Options{})
}

// chatCompletionWith 与 chatCompletion 相同，额外支持两项按调用定制：
//   - display 非空时，正文不逐段推送，流结束后先规整再送入「核心智能 实况」；
//   - loopOpts 指定重复检测参数（零值字段回退到默认阈值）。
//
// 除重复输出外，对模型端的瞬时故障（HTTP 5xx、网络失败）也做有限退避重试：
// 本地/中转模型偶尔连续 500，直接放弃会让审核、评论整环节点失效。
func chatCompletionWith(messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	for attempt := 0; ; attempt++ {
		text, err := chatCompletionOnce(messages, display, loopOpts)
		if err == nil {
			return text, nil
		}
		if errors.Is(err, loopguard.ErrLoop) {
			if attempt >= lifeLoopMaxRetries {
				logger.Error("BilibiliViewer", "模型连续 %d 次陷入重复输出，跳过本次调用", attempt+1)
				return "", fmt.Errorf("模型连续 %d 次陷入重复输出，已跳过本次调用: %w", attempt+1, loopguard.ErrLoop)
			}
			logger.Warn("BilibiliViewer", "检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt+1, lifeLoopMaxRetries)
			continue
		}
		if isTransientModelError(err) {
			if attempt >= modelHTTPRetries {
				logger.Error("BilibiliViewer", "模型连续 %d 次瞬时错误，跳过本次调用: %v", attempt+1, err)
				return "", err
			}
			wait := time.Duration(attempt+1) * modelRetryBackoff
			logger.Warn("BilibiliViewer", "模型瞬时错误（重试 %d/%d，等待 %v）: %v",
				attempt+1, modelHTTPRetries, wait, err)
			time.Sleep(wait)
			continue
		}
		return "", err
	}
}

// isTransientModelError 判断是否为值得重试的瞬时错误：网络失败或 HTTP 5xx。
// 模型端 4xx（鉴权、参数问题）重试无意义，直接返回。
func isTransientModelError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "模型请求失败") {
		return true
	}
	return strings.Contains(msg, "模型 HTTP 5")
}

// chatCompletionOnce 单次模型调用（流式输出受思考监管约束）
func chatCompletionOnce(messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	return chatCompletionOnceModel(modelName, messages, display, loopOpts)
}

// chatCompletionWithModel 用指定模型执行一次带思考监管的调用。
//
// 哔哩观影按阶段切换模型：印象整理用 light_reasoning，审核/评价/动态用
// primary_reasoning，并在切换时主动卸载另一侧释放显存。
func chatCompletionWithModel(model string, messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	for attempt := 0; ; attempt++ {
		text, err := chatCompletionOnceModel(model, messages, display, loopOpts)
		if err == nil {
			return text, nil
		}
		if errors.Is(err, loopguard.ErrLoop) {
			if attempt >= lifeLoopMaxRetries {
				return "", fmt.Errorf("模型连续 %d 次陷入重复输出，已跳过本次调用: %w", attempt+1, loopguard.ErrLoop)
			}
			logger.Warn("BilibiliViewer", "模型[%s]重复输出，重新生成（%d/%d）", model, attempt+1, lifeLoopMaxRetries)
			continue
		}
		if isTransientModelError(err) {
			if attempt >= modelHTTPRetries {
				return "", err
			}
			wait := time.Duration(attempt+1) * modelRetryBackoff
			logger.Warn("BilibiliViewer", "模型[%s]瞬时错误（重试 %d/%d）: %v", model, attempt+1, modelHTTPRetries, err)
			time.Sleep(wait)
			continue
		}
		return "", err
	}
}

// unloadModel 主动卸载指定模型释放显存；失败只告警（下次调用会按需重载）。
func unloadModel(model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	if _, err := llama.UnloadModelByID(model); err != nil {
		logger.Warn("BilibiliViewer", "卸载模型[%s]失败（不影响流程）: %v", model, err)
		return
	}
}

// chatCompletionOnceModel 指定模型名的单次调用（流式输出受思考监管约束）
func chatCompletionOnceModel(model string, messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	observeBegin()
	defer observeEnd()
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
		// 让服务端在流末回传 usage，模型计量才拿得到真实 token
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return "", fmt.Errorf("序列化模型请求失败: %w", err)
	}
	// 命中重复输出时由思考监管取消本次请求，服务端随连接断开停止生成
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
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
		guard := loopguard.New(cancel, loopOpts)
		guard.Start()
		text, reasoning, usage, readErr := readModelStream(resp.Body, display, guard)
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
	// 非流式响应（少数后端忽略 stream）同样做一次静态重复检查
	if loopguard.Detected(text, loopOpts) {
		return "", loopguard.ErrLoop
	}
	observeContent(text, display)
	reportModelCall(body, text+out.Choices[0].Message.ReasoningContent, out.Usage, time.Since(start))
	return text, nil
}

// readModelStream 解析 OpenAI 兼容 SSE，边收边喂给「核心智能 实况」；同时取回思考文本与用量。
// 每段增量同步送入思考监管守卫，命中循环时请求会被取消，读取随之中断。
// display 非空时正文不逐段推送，而是流结束后先规整再推送（编排类调用的输出格式固定）。
func readModelStream(body io.Reader, display func(string) string, guard *loopguard.Guard) (string, string, *modelUsage, error) {
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
				guard.Feed(loopguard.KindThink, choice.Delta.ReasoningContent)
				observeDelta("think", choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				guard.Feed(loopguard.KindContent, choice.Delta.Content)
				if display == nil {
					observeDelta("content", choice.Delta.Content)
				}
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
	// display 为 nil 时正文已逐段推送；非 nil 时整段缓冲到现在，统一规整一次再推送
	if display != nil {
		observeContent(text, display)
	}
	return text, reasoning.String(), usage, nil
}

func chatText(system, user string) (string, error) {
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// chatTextModel 用指定模型跑一次纯文本对话。
func chatTextModel(model, system, user string) (string, error) {
	return chatCompletionWithModel(model, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil, loopguard.Options{})
}

// chatWithImagesModel 用指定模型跑一次图像理解（单次，不重试）。
func chatWithImagesModel(model, system, prompt string, images []string) (string, error) {
	return chatCompletionOnceModel(model, imageMessages(system, prompt, images), nil, loopguard.Options{})
}

// chatTextWithDisplay 同 chatText，但正文先按 display 规整再送入实况。
// 编排类调用的输出是约定格式（如 flow=... 与 share=...），直接原样推送会挤在一起。
func chatTextWithDisplay(system, user string, display func(string) string) (string, error) {
	return chatCompletionWith([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, display, loopguard.Options{})
}

func chatWithImages(system, prompt string, images []string) (string, error) {
	return chatCompletion(imageMessages(system, prompt, images))
}

// chatWithImagesOnce 单次图像理解（不重试）：思考监管命中重复时直接返回 loopguard.ErrLoop。
// 切片理解用它——某个切片反复触发重复时，整条视频的画面理解都不再继续，
// 由调用方改走标题/封面/简介/评论/弹幕的侧面理解。
// 重复检测使用统一的标定参数（loopguard 默认：1024 字节点 / 8 段 128 字 / 平均 0.65）。
func chatWithImagesOnce(system, prompt string, images []string) (string, error) {
	return chatCompletionOnce(imageMessages(system, prompt, images), nil, loopguard.Options{})
}

// imageMessages 组装图像理解请求消息
func imageMessages(system, prompt string, images []string) []chatMessage {
	parts := make([]contentPart, 0, len(images)+1)
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: image}})
	}
	return []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
