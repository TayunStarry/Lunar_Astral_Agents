package ltp9

// ==== 模型垄断：单一虚拟模型池 ====
// 模型垄断是本次移植最重要的差异化设计：无论插件给 engine.llm 传什么模型参数
// （baseUrl/apiKey/model/taskType/selectionStrategy/model_list/ltp9_models.json 等），
// Go 层一律丢弃，强制使用琉璃 agent 字段模型：
//   - 对话/多模态 → GeneralConfig.AgentMultimodal{Model,URL,Key}
//   - 嵌入 → GeneralConfig.AgentEmbedding{Model,URL,Key}
//
// 所有任务类型（replyer/planner/tool_use/vlm/voice/embedding）共享同一个虚拟候选
// = agent 对话模型；getConfig/getAllConfigs/listTasks/getAvailableModels/getCallRecords/
// getCircuitBreakers 都围绕该单模型维护。ltp9_models.json 不参与任何路由。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// agentChatModel 返回 agent 对话模型的 (name, url, key)（强制来源，忽略插件参数）。
func agentChatModel() (name, url, key string) {
	chatCfg()
	return chatModel, chatURL, chatKey
}

// agentEmbedModel 返回 agent 嵌入模型的 (name, url, key)。
func agentEmbedModel() (name, url, key string) {
	chatCfg()
	return embedModel, embedURL, embedKey
}

// monopolyOpts 复制一份 opts 并把模型相关字段强制改写为 agent 字段模型（模型垄断）。
func monopolyOpts(opts map[string]any, model, url, key string) map[string]any {
	out := make(map[string]any, len(opts)+3)
	for k, v := range opts {
		out[k] = v
	}
	out["model"] = model
	out["baseUrl"] = url
	out["apiKey"] = key
	return out
}

// mapGetStr 从 map 安全取字符串。
func mapGetStr(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

// mapBoolTrue 判断 map 中布尔键是否为 true。
func mapBoolTrue(m map[string]any, k string) bool {
	if m == nil {
		return false
	}
	if b, ok := m[k].(bool); ok {
		return b
	}
	return false
}

// extractTools 从 opts 提取 tools（[]any → 透传）。
func extractTools(opts map[string]any) []any {
	if tools, ok := opts["tools"].([]any); ok && len(tools) > 0 {
		return tools
	}
	return nil
}

// extractEmbedTexts 提取待嵌入文本（input 位置入参优先，opts.text/texts 兜底）。
func extractEmbedTexts(input any, opts map[string]any) []string {
	texts := []string{}
	switch v := input.(type) {
	case string:
		texts = append(texts, v)
	case []any:
		for _, s := range v {
			if st, ok := s.(string); ok && st != "" {
				texts = append(texts, st)
			}
		}
	}
	if len(texts) == 0 && opts != nil {
		if t, ok := opts["text"].(string); ok && t != "" {
			texts = []string{t}
		} else if arr, ok := opts["texts"].([]any); ok {
			for _, s := range arr {
				if st, ok := s.(string); ok && st != "" {
					texts = append(texts, st)
				}
			}
		}
	}
	return texts
}

// ==== engine.llm.chat（模型垄断路由） ====

// engineLLMChat 模型垄断对话：丢弃插件模型参数，走到单一 agent 模型并经熔断/记账。
func engineLLMChat(messages []any, opts map[string]any) (any, error) {
	model, url, key := agentChatModel()
	taskType, _ := opts["taskType"].(string) // 名义任务类型（仅用于记账，不参与选模型）

	cb := getCircuitBreaker(model)
	if aerr := cb.allow(); aerr != nil {
		downstreamRecord(model, creatorProviderName, taskType, 0, 0, 0, 0, false)
		return rwResult{Success: false, Error: "模型熔断中: " + aerr.Error()}, nil
	}

	start := time.Now()

	// 1) 宿主注入下层传输（若已注入）：用垄断后的 opts 调用，确保仍走 agent 模型。
	modelInvokerMu.RLock()
	inv := modelInvoker
	modelInvokerMu.RUnlock()

	var content string
	var toolCalls []any
	var pt, ct int
	var cerr error

	if inv != nil {
		res, ierr := inv(messages, monopolyOpts(opts, model, url, key))
		cerr = ierr
		if resm, ok := res.(map[string]any); ok {
			content = mapGetStr(resm, "text")
			if toolList, tok := resm["tool_calls"].([]any); tok {
				toolCalls = toolList
			}
			if !mapBoolTrue(resm, "success") {
				msg := mapGetStr(resm, "error")
				if msg != "" && cerr == nil {
					cerr = fmt.Errorf("%s", msg)
				}
			}
		}
	} else {
		// 2) 默认：琉璃 chatComplete 基板客户端（agent 模型）。
		content, toolCalls, pt, ct, cerr = monoChatComplete(messages, extractTools(opts), opts)
		// 兜底：质管客户端用 model==chatModel 已在内部强制，无需再改
		_ = model
	}

	latency := time.Since(start).Milliseconds()
	var tps float64
	if pt+ct > 0 && latency > 0 {
		tps = float64(pt+ct) / (float64(latency) / 1000.0)
	}
	if cerr != nil {
		cb.recordFailure(isRetryableError(cerr))
		downstreamRecord(model, creatorProviderName, taskType, pt, ct, latency, tps, false)
		return rwResult{Success: false, Error: cerr.Error()}, nil
	}
	cb.recordSuccess()
	downstreamRecord(model, creatorProviderName, taskType, pt, ct, latency, tps, true)
	res := rwResult{Success: true, Text: content}
	if len(toolCalls) > 0 {
		res.ToolCalls = toolCalls
	}
	return res, nil
}

// monoChatComplete 用 agent 对话模型发起一次 chat/completions，返回内容/工具调用/用量。
func monoChatComplete(messages []any, tools []any, opts map[string]any) (string, []any, int, int, error) {
	model, url, key := agentChatModel()
	// 模型垄断：model 固定为 agent 字段模型，opts 里的 baseUrl/apiKey/model 一律被丢弃。
	payload := map[string]any{"model": model, "messages": messages, "stream": false}
	if t, ok := opts["temperature"].(float64); ok {
		payload["temperature"] = t
	}
	if mt, ok := opts["max_tokens"].(int64); ok {
		payload["max_tokens"] = mt
	} else if f, ok := opts["max_tokens"].(float64); ok {
		payload["max_tokens"] = int64(f)
	}
	if sys, ok := opts["system"].(string); ok && sys != "" {
		prepended := make([]any, 0, len(messages)+1)
		prepended = append(prepended, map[string]any{"role": "system", "content": sys})
		prepended = append(prepended, messages...)
		payload["messages"] = prepended
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, 0, 0, fmt.Errorf("请求体序列化失败: %v", err)
	}
	req, rerr := http.NewRequest(http.MethodPost, url+"/chat/completions", bytes.NewReader(body))
	if rerr != nil {
		return "", nil, 0, 0, rerr
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, derr := client.Do(req)
	if derr != nil {
		return "", nil, 0, 0, fmt.Errorf("LLM 请求失败: %v", derr)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, 0, 0, fmt.Errorf("LLM 状态 %d: %s", resp.StatusCode, previewStr(rb, 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   any   `json:"content"`
				ToolCalls []any `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(rb, &out) != nil {
		return "", nil, 0, 0, fmt.Errorf("LLM 响应解析失败")
	}
	if out.Error != nil {
		return "", nil, 0, 0, fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", nil, 0, 0, fmt.Errorf("LLM 无输出")
	}
	return modelContentText(out.Choices[0].Message.Content),
		out.Choices[0].Message.ToolCalls,
		out.Usage.PromptTokens,
		out.Usage.CompletionTokens, nil
}

// ==== engine.llm.embed（模型垄断路由） ====

// engineLLMEmbed 模型垄断嵌入：强制使用 agent 嵌入模型。
func engineLLMEmbed(input any, opts map[string]any) (any, error) {
	texts := extractEmbedTexts(input, opts)
	if len(texts) == 0 {
		return rwResult{Success: false, Error: "缺少待嵌入文本（需传入 string / []string，或 opts.text / opts.texts）"}, nil
	}
	model, url, key := agentEmbedModel()

	// 宿主注入下层传输（若已注入）：用垄断后的 opts 调用。
	embedInvokerMu.RLock()
	inv := embedInvoker
	embedInvokerMu.RUnlock()
	if inv != nil {
		return inv(input, monopolyOpts(opts, model, url, key))
	}

	if url == "" {
		return rwResult{Success: false, Error: "未配置 agent.embedding_url（lunar_config.json）"}, nil
	}
	vecs, err := embedTexts(texts)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if len(vecs) == 1 {
		return map[string]any{"success": true, "embedding": vecs[0]}, nil
	}
	return map[string]any{"success": true, "embeddings": vecs}, nil
}

// ==== 模型池查询（模型垄断下的单模型概要） ====

// poolListTasks 返回 LTP9 支持的任务类型（6 个，全部共享 agent 对话模型）。
func poolListTasks() any {
	return append([]string(nil), monoTasks...)
}

// monoModelSummary 返回指定任务的 agent 单模型概要（task_type 保留入参名义）。
func monoModelSummary(taskType string) map[string]any {
	model, url, _ := agentChatModel()
	return map[string]any{
		"task_type":          taskType,
		"selection_strategy": "monopoly",
		"temperature":        0,
		"max_tokens":         0,
		"models": []any{
			map[string]any{
				"model_identifier": model,
				"name":             model,
				"api_provider":     creatorProviderName,
				"base_url":         url,
				"visible_groups":   []any{},
			},
		},
	}
}

// poolGetConfig 返回指定任务的模型池配置概要（模型垄断：恒为 agent 单模型）。
func poolGetConfig(taskType string) any {
	return monoModelSummary(taskType)
}

// poolGetAllConfigs 返回全部任务类型的配置概要。
func poolGetAllConfigs() any {
	out := map[string]any{}
	for _, t := range monoTasks {
		out[t] = monoModelSummary(t)
	}
	return out
}

// poolGetAvailableModels 返回 agent 对话模型名（唯一候选）。
func poolGetAvailableModels() any {
	model, _, _ := agentChatModel()
	return []any{model}
}

// poolCircuitStatus 返回全部模型熔断器状态（模型垄断下即 agent 单模型熔断器）。
func poolCircuitStatus() any {
	return getAllCircuitBreakers()
}