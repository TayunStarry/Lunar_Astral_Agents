package WebLTP

// Web-LTP 模型调用（OpenAI v1 协议，复用月华多模态模型配置）

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoggerGeneralWarn 统一日志入口
func LoggerGeneralWarn(format string, args ...any) {
	LoggerGeneral.Warn("CrystalAstral", format, args...)
}

var (
	modelOnce  sync.Once
	modelName  string
	modelURL   string
	modelKey   string
	httpClient = &http.Client{Timeout: 120 * time.Second}

	// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End，
	// 让 TUI「核心智能 实况」能看到网络搜索的模型思考与输出。
	observerMu    sync.Mutex
	observerBegin func(role string)
	observerDelta func(role, kind, text string)
	observerEnd   func(role string)
)

// webSearchModelRole 网络搜索的实况角色标签（TUI 显示）
const webSearchModelRole = "网络搜索"

// SetModelObserver 注入实况观察者（app 层启动时调用）
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string)) {
	observerMu.Lock()
	defer observerMu.Unlock()
	observerBegin, observerDelta, observerEnd = begin, delta, end
}

// observeBegin 通知实况开始
func observeBegin() {
	observerMu.Lock()
	fn := observerBegin
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole)
	}
}

// observeDelta 通知实况增量（kind: think/content）
func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := observerDelta
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole, kind, text)
	}
}

// observeEnd 通知实况结束
func observeEnd() {
	observerMu.Lock()
	fn := observerEnd
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole)
	}
}

func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(*GeneralConfig.AgentMultimodalModel)
		if modelName == "" {
			modelName = "system-multimodal"
		}
		modelURL = strings.TrimRight(strings.TrimSpace(*GeneralConfig.AgentMultimodalURL), "/")
		if modelURL == "" {
			modelURL = GeneralConfig.LocalV1URL()
		}
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
		modelKey = strings.TrimSpace(*GeneralConfig.AgentMultimodalKey)
	})
}

// textWithImages 构建图文混排的用户消息内容（无图片时退化为纯文本）
func textWithImages(text string, dataURLs []string) []contentPart {
	parts := []contentPart{{Type: "text", Text: text}}
	for _, dataURL := range dataURLs {
		if dataURL == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: dataURL}})
	}
	return parts
}

// chatText 发起一次对话并返回助手文本（流式，思考与输出同步进实况屏）
func chatText(messages []chatMessage) (string, error) {
	observeBegin()
	defer observeEnd()
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateRunes(string(respBody), 200))
	}
	// 服务端可能忽略 stream 参数直接返回 JSON：两种形态都要兼容
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readModelStream(resp.Body)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("模型 API 错误: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("模型 API 返回空响应")
	}
	text := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	observeDelta("content", text)
	return text, nil
}

// readModelStream 解析 OpenAI 兼容 SSE，边收边送实况
func readModelStream(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content strings.Builder
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
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", fmt.Errorf("模型 API 错误: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				observeDelta("think", choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				observeDelta("content", choice.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取模型流失败: %w", err)
	}
	text := strings.TrimSpace(content.String())
	if text == "" {
		return "", fmt.Errorf("模型 API 返回空响应")
	}
	return text, nil
}

// ExtractQuery 把月华的自然语言指令提炼为必应检索词；失败时返回原指令。
func ExtractQuery(instruction string) string {
	q, err := chatText([]chatMessage{
		{Role: "system", Content: loadPrompt("webSearchExtractQuery.md")},
		{Role: "user", Content: instruction},
	})
	if err != nil {
		return instruction
	}
	q = strings.TrimSpace(q)
	if q == "" || len([]rune(q)) > 120 {
		return instruction
	}
	return strings.Trim(q, "\"“”「」")
}

// hardCutRunes 硬切断文本：总长不超过 max 字符（超长时以省略号收尾并标注）
func hardCutRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	return string(r[:max-1]) + "…（已截断）"
}

// truncateRunes 简单截断（用于日志/载荷）
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
