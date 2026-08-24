package AgentSearch

import (
	"LunarSubsystem/GeneralConfig"
	"bytes"
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
	aiSummarizeContent = summarizeContent
	aiJudgeMemory = judgeMemory
	aiGenerateReport = generateReport
	aiSummarizeVisualContent = summarizeVisualContent
	// 统一网络搜索钩子
	aiEnhanceSearchText = enhanceSearchText
	// 新流程：关键词+实体提取、综合判定
	aiExtractKeywords = extractKeywordsAndEntities
	aiJudgeComprehensive = judgeComprehensive
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

	return callChatAPI(*GeneralConfig.SearchMultimodalURL, *GeneralConfig.SearchMultimodalModel, *GeneralConfig.SearchMultimodalKey, messages, maxTokens)
}

// callEmbedding 调用嵌入 API，返回文本的嵌入向量
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
func callEmbedding(text string) ([]float32, error) {
	body := embeddingRequest{
		Model: *GeneralConfig.SearchEmbeddingModel,
		Input: text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化嵌入请求失败: %w", err)
	}

	baseURL := normalizeBaseURL(*GeneralConfig.SearchEmbeddingURL)
	req, err := http.NewRequest("POST", baseURL+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建嵌入请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if *GeneralConfig.SearchEmbeddingKey != "" {
		req.Header.Set("Authorization", "Bearer "+*GeneralConfig.SearchEmbeddingKey)
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

// parseYesNoResponse 解析 AI 的"是/否"判定响应（兼容中英文冒号）
func parseYesNoResponse(resp string) bool {
	lower := strings.ToLower(resp)
	if strings.Contains(lower, "是") && !strings.Contains(lower, "否") {
		return true
	}
	return false
}

// enhanceSearchText 推测用户真实意图，产出一条【强化后的搜索词】
// 结合上一轮未能解答的摘要与相关历史记忆提示，避开方向，转向可能命中的新角度
func enhanceSearchText(query string, priorSummaries string, memoryHints string) (string, error) {
	systemPrompt := `你是一个搜索意图分析与关键词优化专家。基于用户的原始请求文本，合理推测用户真正想查的是什么，产出一条强化后的搜索词，用于解决搜索引擎对原始表述理解偏差、结果不相关的问题。

要求：
1. 只输出一条搜索词，不要编号、不要引号、不要多余文字
2. 保留原始请求中的核心实体/专有名词和关键限定词
3. 去除"查询/帮我查/我想知道"等口语套话，提炼成更精确、利于搜索引擎命中的短语
4. 结合"上一轮未能解答的内容"与"相关历史记忆提示"，避免再撞上相同方向，补充地点/年代/分类等限定词转向可能命中的新角度
5. 若记忆提示显示某方向已尝试且失败，就不要沿用该方向`

	userPrompt := fmt.Sprintf(
		"用户的原始请求：%s\n\n上一轮已尝试的页面摘要（未能解答）：\n%s\n\n相关历史记忆提示：\n%s\n\n请输出一条强化后的搜索词：",
		query, truncateText(priorSummaries, 1500), truncateText(memoryHints, 1500))
	return callAI(systemPrompt, userPrompt, nil)
}

// =============================================================================
// Hook 实现：关键词 + 核心实体提取（新流程步骤1）
// =============================================================================

// keywordExtractionResult 关键词提取的 JSON 响应结构
type keywordExtractionResult struct {
	Entities []string `json:"entities"`
	Keywords []string `json:"keywords"`
}

// extractKeywordsAndEntities 从用户查询中提取【核心完整实体名】与【搜索关键词数组】
// entities：用于标题初筛和摘要关键词精确比对的完整实体名词
// keywords：用于空格拼接成初始查询语句
func extractKeywordsAndEntities(query string) ([]string, []string, error) {
	systemPrompt := `你是一个搜索关键词与实体提取专家。从用户问题中提取两类信息，输出 JSON。

1. entities（核心实体）：问题指向的【独立的、简短的】专有名词。
   - 每个元素是一个独立专名（通常 2-5 个汉字），多个核心对象要拆成多个数组元素。例如"南京南站""原神""终末地"各自是一个元素。
   - 严禁把一句话或一个短语塞进一个元素，严禁用空格/标点把多个词连成一个元素。
   - 只保留实体名本身，不要带"模组/小说/游戏/卡池/下载/介绍/信息/最新"等类别词或泛化词；这些词放到 keywords。
   - 遇到自造昵称/网名/作品名要整体保留（如"钛宇星光阁"不要拆字）。
2. keywords（搜索关键词）：用于拼接查询语句的词（包含核心实体与必要限定词，如"最新""卡池""在哪里""介绍"）。

输出格式（只输出 JSON，不要任何多余文字）：
{"entities":["实体1","实体2"],"keywords":["关键词1","关键词2","关键词3"]}`

	userPrompt := fmt.Sprintf("用户问题：%s", query)
	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return nil, nil, err
	}

	entities, keywords := parseKeywordExtractionResponse(resp)
	return entities, keywords, nil
}

// parseKeywordExtractionResponse 解析关键词提取 JSON，兼容 markdown 代码块包裹与非严格 JSON
func parseKeywordExtractionResponse(resp string) ([]string, []string) {
	clean := strings.TrimSpace(resp)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start >= 0 && end > start {
		clean = clean[start : end+1]
	}

	var res keywordExtractionResult
	if err := json.Unmarshal([]byte(clean), &res); err != nil {
		// 解析失败时回退：整行作为关键词，实体留空（标题初筛自动放行）
		return nil, parseKeywordResponse(resp)
	}

	return res.Entities, res.Keywords
}

// =============================================================================
// Hook 实现：综合判定（新流程步骤8）
// =============================================================================

// judgeComprehensive 综合判定多份网页摘要拼接后是否足以解答用户问题
// 放宽判定标准：主体信息能回答核心诉求即可，不要求百分百完整，避免"有答案却说不知道"
func judgeComprehensive(query string, memoryReference string, summaries string) (bool, error) {
	systemPrompt := `你是一个搜索答案综合判定专家。判断给出的多份网页摘要拼接后，是否足以解答用户的问题。

判定"能解答"的标准：
- 信息覆盖了问题的核心诉求（对象本身、位置、历史、构成、数据、最新动态等关键点）→ 能解答
- 只要主体信息能回答用户问题，即使部分细节缺失，也算能解答（不要过分要求完整）
- 信息与用户问题主题不同、只是泛泛背景或工具页、不足以支撑回答 → 不能解答
- 相关历史记忆仅供参考，若与当前问题不符应忽略

请严格按以下格式回复（仅一行，不要多余内容）：
解答：是/否`

	userPrompt := fmt.Sprintf("用户问题：%s\n\n网页摘要（拼接）：\n%s\n\n相关历史记忆（供参考）：\n%s",
		query, truncateText(summaries, 4000), truncateText(memoryReference, 1500))
	resp, err := callAI(systemPrompt, userPrompt, nil)
	if err != nil {
		return false, err
	}
	return parseYesNoResponse(resp), nil
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

判断1：历史记录是否【直接、明确】回答了用户的同一个问题/同一个对象（同一地点、人物、作品、事件、产品）？
- 只有当历史记录所问的东西与用户现在问的是同一个特定对象，且答案确实覆盖了提问内容时才判为"是"
- 仅是与问题"背景相似"或"提到同一个城市"但主题不同的记录，不算直接回答，判为"否"
- 例如：用户现在问"牛首山"，历史记录里只有"南京南站"或"南京旅游汇总"，不算直接回答

判断2：用户问题是否有时效性要求，需要最新的信息？（是/否）
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
