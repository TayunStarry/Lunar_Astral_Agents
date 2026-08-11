package lunar_chromedp

import (
	"bytes"
	"config"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// =============================================================================
// AI 调用层 — OpenAI 兼容 API（Chat Completion + Embedding）
// =============================================================================

func init() {
	// 注册 AI 调用钩子到 agent.go
	aiCall = callAI
	aiGenerateKeywords = generateKeywords
	aiGenerateDeepKeywords = generateDeepKeywords
	aiSummarizeContent = summarizeContent
	aiJudgeMemory = judgeMemory
	aiEvaluateSufficiency = evaluateSufficiency
	aiGenerateReport = generateReport
	// 快速搜索模式钩子
	aiDecideSearchMode = decideSearchMode
	aiSummarizeVisualContent = summarizeVisualContent
}

// =============================================================================
// 核心 AI 调用函数
// =============================================================================

// callAI 通用 AI 调用：发送 system + user prompt（可选图片），返回文本响应
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
func callAI(systemPrompt string, userPrompt string, images [][]byte) (string, error) {
	maxTokens := MaxContextTokensDefault
	configMutex.RLock()
	if activeConfig != nil && activeConfig.MaxContextTokens > 0 {
		maxTokens = activeConfig.MaxContextTokens
	}
	configMutex.RUnlock()

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
	}

	// 构建用户消息（支持多模态）
	if len(images) > 0 {
		parts := make([]contentPart, 0, len(images)+1)
		if userPrompt != "" {
			parts = append(parts, contentPart{Type: "text", Text: userPrompt})
		}
		for _, img := range images {
			b64 := base64.StdEncoding.EncodeToString(img)
			parts = append(parts, contentPart{
				Type: "image_url",
				ImageURL: &imageURL{
					URL: "data:image/png;base64," + b64,
				},
			})
		}
		messages = append(messages, chatMessage{Role: "user", Content: parts})
	} else {
		messages = append(messages, chatMessage{Role: "user", Content: userPrompt})
	}

	return callChatAPI(*config.SearchMultimodalURL, *config.SearchMultimodalModel, *config.SearchMultimodalKey, messages, maxTokens)
}

// callEmbedding 调用嵌入 API，返回文本的嵌入向量
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
func callEmbedding(text string) ([]float32, error) {
		body := embeddingRequest{
			Model: *config.SearchEmbeddingModel,
			Input: text,
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化嵌入请求失败: %w", err)
		}

		baseURL := normalizeBaseURL(*config.SearchEmbeddingURL)
		req, err := http.NewRequest("POST", baseURL+"/embeddings", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("创建嵌入请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if *config.SearchEmbeddingKey != "" {
			req.Header.Set("Authorization", "Bearer "+*config.SearchEmbeddingKey)
		}

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("嵌入请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取嵌入响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("嵌入 API 返回状态 %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("嵌入 API 错误: %s", embResp.Error.Message)
	}

	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("嵌入 API 返回空向量")
	}

	return embResp.Data[0].Embedding, nil
}

// callChatAPI 底层聊天 API 调用
func callChatAPI(baseURL, modelName, apiKey string, messages []chatMessage, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = MaxContextTokensDefault
	}

	body := chatRequest{
		Model:       modelName,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: 0.3,
		Stream:      false,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("序列化聊天请求失败: %w", err)
	}

	normalizedURL := normalizeBaseURL(baseURL)
	req, err := http.NewRequest("POST", normalizedURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("创建聊天请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("聊天请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取聊天响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("聊天 API 返回状态 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析聊天响应失败: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("聊天 API 错误: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("聊天 API 返回空响应")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// =============================================================================
// Hook 实现：关键词生成
// =============================================================================

// generateKeywords 将自然语言查询转化为搜索关键词
func generateKeywords(query string) ([]string, error) {
	systemPrompt := `你是一个搜索关键词优化专家。将用户的问题转化为1-3个精确的搜索引擎关键词。

规则：
1. 每个关键词应为独立的搜索短语，用换行分隔
2. 提取核心概念，去除冗余修饰词
3. 优先使用英文关键词（如果问题本身是英文或技术类）
4. 对不同角度的查询，生成不同侧重点的关键词
5. 不要添加编号、引号或其他格式标记

示例：
用户问："最新的Go语言Web框架有哪些？"
输出：
best Go web frameworks 2026
Go web framework comparison
Golang HTTP router library`

	resp, err := callAI(systemPrompt, query, nil)
	if err != nil {
		return nil, err
	}

	return parseKeywordResponse(resp), nil
}

// generateDeepKeywords 基于已有上下文生成深度搜索关键词（含嵌入向量去重）
func generateDeepKeywords(query string, accumulatedSummaries string, usedKeywords []string) ([]string, error) {
	// 截断上下文防止超出 token 限制
	contextSummary := truncateText(accumulatedSummaries, 2000)

	systemPrompt := `你是一个深度搜索策略专家。根据用户原始问题和已经获取的信息，生成1-2个新的搜索关键词，从不同角度补充缺失的信息。

规则：
1. 生成的关键词必须是之前搜索没有覆盖的角度
2. 用换行分隔每个关键词
3. 不要重复已有信息中已经包含的内容方向
4. 聚焦于"还缺少什么信息"来生成补充性搜索词
5. 不要添加编号或其他格式标记`

	userPrompt := fmt.Sprintf("用户问题：%s\n\n已获取的信息摘要：\n%s\n\n已使用的关键词：%s\n\n请生成新的补充搜索关键词：",
		query, contextSummary, strings.Join(usedKeywords, ", "))

	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return nil, err
	}

	candidates := parseKeywordResponse(resp)
	if len(candidates) == 0 {
		return nil, nil
	}

	// 嵌入向量去重：拒绝与已用关键词余弦相似度 > 0.85 的候选词
	return dedupKeywordsByEmbedding(candidates)
}

// =============================================================================
// Hook 实现：内容摘要
// =============================================================================

// summarizeContent 对网页内容进行 AI 摘要
func summarizeContent(textContent string, screenshots [][]byte) (string, error) {
	if len(screenshots) > 0 {
		// 多模态摘要：文本 + 截图
		systemPrompt := `你是一个网页内容分析专家。根据提供的网页文本和截图，生成一段简洁的内容摘要。

要求：
1. 提取网页的核心信息和关键事实
2. 摘要长度控制在200字以内
3. 使用中文输出
4. 如果网页内容与提供的文本无关，注明"内容不相关"
5. 以客观第三人称描述，不添加主观评价`

		userPrompt := fmt.Sprintf("网页文本内容：\n%s\n\n请基于以上文本和截图生成摘要。", truncateText(textContent, 3000))
		return callAI(systemPrompt, userPrompt, screenshots)
	}

	// 纯文本摘要
	if strings.TrimSpace(textContent) == "" {
		return "（网页无有效文本内容）", nil
	}

	systemPrompt := `你是一个网页内容摘要专家。根据提供的网页文本，生成一段简洁的内容摘要。

要求：
1. 提取网页的核心信息和关键事实
2. 摘要长度控制在200字以内
3. 使用中文输出
4. 以客观第三人称描述，不添加主观评价`

	userPrompt := fmt.Sprintf("网页内容：\n%s\n\n请生成摘要。", truncateText(textContent, 4000))
	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp), nil
}

// =============================================================================
// Hook 实现：纯视觉内容摘要（快速搜索专用）
// =============================================================================

// summarizeVisualContent 仅基于页面截图生成内容摘要，不接收文本
// 用于快速搜索模式：跳过 DOM 文本提取，直接通过多模态模型理解页面视觉内容
func summarizeVisualContent(screenshots [][]byte) (string, error) {
	if len(screenshots) == 0 {
		return "（页面无截图内容）", nil
	}

	systemPrompt := `你是一个网页视觉内容分析专家。根据提供的网页截图，生成一段简洁的页面内容摘要。

要求：
1. 从截图中提取页面展示的核心信息和关键内容
2. 摘要长度控制在200字以内
3. 使用中文输出
4. 如果截图无法辨认有效内容，注明"页面内容不清晰"
5. 以客观第三人称描述，不添加主观评价
6. 如果截图显示的是错误页、404、验证码等，直接说明`

	userPrompt := "请基于以上网页截图生成内容摘要。"
	resp, err := callAI(systemPrompt, userPrompt, screenshots)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp), nil
}

// =============================================================================
// Hook 实现：记忆判定
// =============================================================================

// judgeMemory 判定记忆库内容是否足以回答用户问题
// 返回：(足够, 有时效性要求, 错误)
func judgeMemory(memoryContext string, query string) (bool, bool, error) {
	systemPrompt := `你是一个信息充分性评估专家。根据历史搜索记录和用户当前问题，做出两个判断：

1. 历史记录中的信息是否足以回答用户问题？（是/否）
2. 用户问题是否有时效性要求，需要最新的信息？（是/否）

时效性判断标准：
- 询问"今天"、"最新"、"当前"、"现在"等时间敏感词 → 是
- 询问新闻、天气、股价、赛事等实时信息 → 是
- 询问概念、原理、历史、教程等静态知识 → 否

请严格按以下格式回复（仅回复两行，不要多余内容）：
足够：是/否
时效性：是/否`

	userPrompt := fmt.Sprintf("用户当前问题：%s\n\n%s", query, memoryContext)
	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return false, false, err
	}

	return parseJudgeResponse(resp)
}

// =============================================================================
// Hook 实现：搜索模式判定（快速搜索 vs 深度搜索）
// =============================================================================

// decideSearchMode 判定用户查询是否适合快速视觉搜索模式
// 快速搜索适合：视觉对比、产品外观、UI设计、页面截图类查询
// 深度搜索适合：研究类、事实核查、多源验证、学术类查询
// 返回：(是否使用快速搜索, 判定理由, 错误)
func decideSearchMode(query string) (bool, string, error) {
	systemPrompt := `你是一个搜索策略专家。根据用户问题，判断最适合的搜索模式。

两种模式说明：
1. 【快速视觉搜索】：直接浏览网页截图，适合视觉类查询（产品外观、设计参考、UI对比、页面截图等）
2. 【深度文本搜索】：提取网页文本并深度分析，适合研究类查询（事实核查、多源验证、学术问题、复杂推理等）

判定标准：
- 问题涉及视觉对比、外观、设计、界面 → 快速视觉搜索
- 问题涉及概念、原理、数据、分析、论证 → 深度文本搜索
- 问题简单直接、不需要深入分析 → 快速视觉搜索
- 问题需要多角度验证、信息整合 → 深度文本搜索

请严格按以下格式回复（仅回复两行，不要多余内容）：
模式：快速/深度
理由：一句话简述判定依据`

	resp, err := callAI(systemPrompt, query, nil)
	if err != nil {
		return false, "", err
	}

	return parseModeDecisionResponse(resp)
}

// parseModeDecisionResponse 解析 AI 的模式判定响应
func parseModeDecisionResponse(resp string) (useQuick bool, reasoning string, err error) {
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "模式：") || strings.HasPrefix(lower, "模式:") {
			useQuick = strings.Contains(lower, "快速")
		}
		if strings.HasPrefix(lower, "理由：") || strings.HasPrefix(lower, "理由:") {
			reasoning = strings.TrimPrefix(line, "理由：")
			reasoning = strings.TrimPrefix(reasoning, "理由:")
			reasoning = strings.TrimSpace(reasoning)
		}
	}
	return useQuick, reasoning, nil
}

// =============================================================================
// Hook 实现：信息充分性评估
// =============================================================================

// evaluateSufficiency 评估当前积累的信息是否足以回答用户问题
func evaluateSufficiency(query string, accumulatedSummaries string) (bool, string, error) {
	systemPrompt := `你是一个信息充分性评估专家。判断当前积累的搜索摘要是否包含了足够的信息来全面回答用户的问题。

评估标准：
- 如果信息覆盖了问题的所有关键方面 → 足够
- 如果存在明显的信息缺口 → 不足

请严格按以下格式回复：
判定：足够/不足
理由：一句话简述判定依据`

	userPrompt := fmt.Sprintf("用户问题：%s\n\n已收集的信息摘要：\n%s", query, truncateText(accumulatedSummaries, 4000))
	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return false, "", err
	}

	return parseSufficiencyResponse(resp)
}

// =============================================================================
// Hook 实现：报告生成
// =============================================================================

// generateReport 基于所有摘要生成最终搜索报告
func generateReport(query string, summaries []string, sources []string) (string, error) {
	// 构建来源列表
	var sourcesText strings.Builder
	for i, src := range sources {
		sourcesText.WriteString(fmt.Sprintf("%d. %s\n", i+1, src))
	}

	// 合并摘要
	combinedSummaries := strings.Join(summaries, "\n\n---\n\n")

	systemPrompt := `你是一个专业的搜索报告生成专家。基于用户问题和收集到的信息摘要，生成一份结构清晰、内容准确的搜索答案报告。

报告格式要求：
1. 【核心答案】：用1-2句话直接回答用户问题
2. 【详细分析】：展开说明关键信息，分点陈述
3. 【信息来源】：列出引用来源编号
4. 【补充说明】：如有必要，补充相关注意事项或延伸信息

要求：
- 语言精炼，避免冗余
- 客观准确，不添加未经证实的信息
- 如果信息不足以完全回答，诚实说明
- 使用中文输出`

	userPrompt := fmt.Sprintf("用户问题：%s\n\n收集到的信息摘要：\n%s\n\n信息来源：\n%s\n\n请生成搜索报告。",
		query, truncateText(combinedSummaries, 6000), sourcesText.String())

	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp), nil
}

// =============================================================================
// 嵌入向量去重
// =============================================================================

// dedupKeywordsByEmbedding 对候选关键词进行嵌入向量去重
// 余弦相似度 > KeywordDedupThreshold 视为与已用关键词重复，拒绝
func dedupKeywordsByEmbedding(candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	var filtered []string

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		// 获取候选词嵌入
		candidateEmb, err := getOrComputeEmbedding(candidate)
		if err != nil {
			fmt.Printf("[%s] 关键词嵌入失败 '%s': %v，保留该关键词\n", ModuleName, candidate, err)
			filtered = append(filtered, candidate)
			continue
		}

		// 与所有已用关键词比较
		isDuplicate := false
		keywordEmbedMu.RLock()
		for usedKW, usedEmb := range keywordEmbedCache {
			sim := cosineSimilarity32(candidateEmb, usedEmb)
			if sim >= float32(KeywordDedupThreshold) {
				fmt.Printf("[%s] 关键词去重: '%s' 与 '%s' 相似度=%.2f，拒绝\n",
					ModuleName, candidate, usedKW, sim)
				isDuplicate = true
				break
			}
		}
		keywordEmbedMu.RUnlock()

		if !isDuplicate {
			filtered = append(filtered, candidate)
			// 缓存通过的关键词嵌入
			keywordEmbedMu.Lock()
			keywordEmbedCache[candidate] = candidateEmb
			keywordEmbedMu.Unlock()
		}
	}

	return filtered, nil
}

// getOrComputeEmbedding 获取关键词的嵌入向量（优先从缓存读取）
func getOrComputeEmbedding(keyword string) ([]float32, error) {
	keywordEmbedMu.RLock()
	if emb, ok := keywordEmbedCache[keyword]; ok {
		keywordEmbedMu.RUnlock()
		return emb, nil
	}
	keywordEmbedMu.RUnlock()

	emb, err := callEmbedding(keyword)
	if err != nil {
		return nil, err
	}

	keywordEmbedMu.Lock()
	keywordEmbedCache[keyword] = emb
	keywordEmbedMu.Unlock()

	return emb, nil
}

// =============================================================================
// 响应解析辅助函数
// =============================================================================

// parseKeywordResponse 解析 AI 返回的关键词列表
func parseKeywordResponse(resp string) []string {
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	var keywords []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 清理可能的编号前缀（"1. "、"1、"、"- " 等）
		line = strings.TrimLeft(line, "0123456789.、-· ")
		line = strings.TrimSpace(line)
		if line != "" {
			keywords = append(keywords, line)
		}
	}
	return keywords
}

// parseJudgeResponse 解析 AI 的记忆判定响应
func parseJudgeResponse(resp string) (sufficient bool, timeSensitive bool, err error) {
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "足够：") || strings.HasPrefix(lower, "足够:") {
			sufficient = strings.Contains(lower, "是")
		}
		if strings.HasPrefix(lower, "时效性：") || strings.HasPrefix(lower, "时效性:") {
			timeSensitive = strings.Contains(lower, "是")
		}
	}
	return sufficient, timeSensitive, nil
}

// parseSufficiencyResponse 解析 AI 的充分性评估响应
func parseSufficiencyResponse(resp string) (sufficient bool, reasoning string, err error) {
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "判定：") || strings.HasPrefix(lower, "判定:") {
			sufficient = strings.Contains(lower, "足够")
		}
		if strings.HasPrefix(lower, "理由：") || strings.HasPrefix(lower, "理由:") {
			reasoning = strings.TrimPrefix(line, "理由：")
			reasoning = strings.TrimPrefix(reasoning, "理由:")
			reasoning = strings.TrimSpace(reasoning)
		}
	}
	return sufficient, reasoning, nil
}

// =============================================================================
// 工具函数
// =============================================================================

// normalizeBaseURL 规范化 API 基础 URL，确保以 /v1 结尾
func normalizeBaseURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	// 检查是否已经包含 /v1 但不是结尾
	if strings.Contains(u, "/v1/") || strings.Contains(u, "/v1?") {
		return u
	}
	return u + "/v1"
}

// cosineSimilarity32 计算两个 float32 向量的余弦相似度
func cosineSimilarity32(a, b []float32) float32 {
	n := len(a)
	if n != len(b) || n == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
