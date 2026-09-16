package YaraLTP

// ==== 模型客户端（OpenAI v1，供 yara.model 使用） ====

import (
	"LunarSubsystem/GeneralConfig"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// chatCfg 惰性加载模型配置（chat / embedding，从 lunar_config.json 读取）。
func chatCfg() {
	modelCfgOnce.Do(func() {
		chatModel = stringOr(*GeneralConfig.AgentMultimodalModel, "system-multimodal")
		chatURL = normV1(stringOr(*GeneralConfig.AgentMultimodalURL, "http://127.0.0.1:36789/v1"))
		chatKey = stringOr(*GeneralConfig.AgentMultimodalKey, "")
		embedModel = stringOr(*GeneralConfig.AgentEmbeddingModel, "system-embedding")
		embedURL = normV1(stringOr(*GeneralConfig.AgentEmbeddingURL, "http://127.0.0.1:36789/v1"))
		embedKey = stringOr(*GeneralConfig.AgentEmbeddingKey, "")
	})
}

// modelChatName 返回当前对话模型名。
func modelChatName() string {
	chatCfg()
	return chatModel
}

// stringOr 返回第一个非空，全空返回默认。
func stringOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// normV1 确保请求地址以 /v1 结尾。
func normV1(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	return u + "/v1"
}

// chatComplete 发起一次对话补全，返回助手文本内容。
func chatComplete(messages, tools []map[string]any, baseURL, apiKey, model string) (string, error) {
	chatCfg()
	url := chatURL
	if baseURL != "" {
		url = normV1(baseURL)
	}
	if model == "" {
		model = chatModel
	}
	if strings.TrimSpace(apiKey) == "" {
		apiKey = chatKey
	}
	if messages == nil {
		messages = []map[string]any{}
	}
	body := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("请求体序列化失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("模型返回状态 %d: %s", resp.StatusCode, truncateStr(string(raw), 300))
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("模型错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("模型返回空响应")
	}
	return modelContentText(cr.Choices[0].Message.Content), nil
}

// modelContentText 把 content（string 或片段数组）转为纯文本。
func modelContentText(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// modelChatFromParams 从 yara.model.chat 的 params 提取并调用。
func modelChatFromParams(p map[string]any) (string, error) {
	messages := mapGetAnyList(p, "messages")
	tools := mapGetAnyList(p, "tools")
	base := mapGetStr(p, "baseUrl")
	key := mapGetStr(p, "apiKey")
	model := mapGetStr(p, "model")
	if len(messages) == 0 {
		return "", fmt.Errorf("缺少 messages 参数")
	}
	return chatComplete(messages, tools, base, key, model)
}

// embedText 单文本取嵌入向量。
func embedText(text string) ([]float64, error) {
	vecs, err := embedTexts([]string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// embedTexts 批量文本取嵌入向量。
func embedTexts(texts []string) ([][]float64, error) {
	chatCfg()
	body := map[string]any{"model": embedModel, "input": texts}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, embedURL+"/embeddings", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if embedKey != "" {
		req.Header.Set("Authorization", "Bearer "+embedKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("嵌入请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("嵌入服务返回状态 %d: %s", resp.StatusCode, truncateStr(string(raw), 200))
	}
	var er struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(raw, &er); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}
	if er.Error != nil {
		return nil, fmt.Errorf("嵌入错误: %s", er.Error.Message)
	}
	out := make([][]float64, 0, len(er.Data))
	for _, d := range er.Data {
		out = append(out, d.Embedding)
	}
	return out, nil
}

// truncateStr 按 rune 截断字符串。
func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}