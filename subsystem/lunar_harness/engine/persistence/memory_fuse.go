package persistence

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// =============================================================================
// 权重融合 — 把一个集合内「超高相似度」的文本记忆合并成一条整合记忆
//
// 仅处理文本记忆：图片/音频记忆没有可比较的正文，也不应被合并（其向量来自标签/描述）。
// 流程：取候选 → 补齐内容向量 → 以余弦相似度做代表式聚类 → 按策略合并正文
//       → 用合并正文重新生成嵌入向量 → 删除原条目并写入整合条目。
// =============================================================================

// MemoryFuseEntries 对指定集合的文本记忆执行权重融合。
//
// ids 为空表示扫描整个集合；ids 非空时只处理这些条目（其余条目可能被合并进同一簇，
// 但只有命中的条目参与候选）。返回值说明本次合并了哪些簇、删了多少条、建了多少条。
func (d *MemoryDB) MemoryFuseEntries(ctx context.Context, collectionName string, ids []string, opts MemoryFuseOptions) (MemoryFuseResult, error) {
	result := MemoryFuseResult{Collection: collectionName, Groups: []MemoryFuseGroup{}}
	normalized, err := NormalizeFuseStrategy(opts.Strategy)
	if err != nil {
		return result, err
	}
	threshold := normalizeFuseThreshold(opts.Threshold)
	result.Threshold = threshold

	c, err := d.getCollection(collectionName)
	if err != nil {
		return result, err
	}
	c.reloadIfChanged()

	// 1. 取候选文本条目（跳过媒体/空正文/显式指定之外的条目）
	c.mu.RLock()
	docs := append([]Document(nil), c.Documents...)
	model := c.Model
	c.mu.RUnlock()

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = true
		}
	}

	candidates := make([]Document, 0, len(docs))
	for _, doc := range docs {
		if len(wanted) > 0 && !wanted[doc.ID] {
			continue
		}
		if memoryDocumentType(doc) != MemoryTypeText || strings.TrimSpace(doc.Content) == "" || doc.URL != "" || doc.Base64 != "" {
			result.SkippedMedia++
			continue
		}
		candidates = append(candidates, doc)
	}
	if len(candidates) > MemoryFuseMaxCandidates {
		return result, fmt.Errorf("本次候选 %d 条，超过单次融合上限 %d 条；请只勾选要融合的条目或分批执行", len(candidates), MemoryFuseMaxCandidates)
	}
	result.CandidateCount = len(candidates)
	if len(candidates) < 2 {
		return result, nil
	}

	if model == "" {
		model = currentMemoryEmbeddingModel()
	}

	// 2. 补齐内容向量（历史条目可能缺向量；维度不一致的条目无法比较，跳过）
	valid := make([]Document, 0, len(candidates))
	for _, doc := range candidates {
		if len(doc.Embedding) == 0 {
			vec, embedErr := d.embedText(ctx, model, doc.Content)
			if embedErr != nil {
				logger.Warn("FileManager", "融合跳过 %s：嵌入失败 %v", doc.ID, embedErr)
				result.SkippedMedia++
				continue
			}
			doc.Embedding = vec
		}
		valid = append(valid, doc)
	}
	if len(valid) < 2 {
		return result, nil
	}
	// 以首条维度为准，避免不同模型产生的向量混算
	dimension := len(valid[0].Embedding)
	aligned := valid[:0]
	for _, doc := range valid {
		if len(doc.Embedding) != dimension {
			result.SkippedMedia++
			continue
		}
		aligned = append(aligned, doc)
	}
	valid = aligned

	// 3. 代表式聚类：与簇代表条目相似度达阈值即并入，避免链式传递把不相关条目串成一簇
	clusters := clusterMemoryDocs(valid, threshold)

	// 4. 逐簇合并（模型/嵌入阶段不持锁）
	type pendingMerge struct {
		removeIDs []string
		doc       Document
		group     MemoryFuseGroup
	}
	pending := make([]pendingMerge, 0, len(clusters))
	for _, cluster := range clusters {
		if len(cluster.indices) < 2 {
			continue
		}
		groupDocs := make([]Document, 0, len(cluster.indices))
		for _, index := range cluster.indices {
			groupDocs = append(groupDocs, valid[index])
		}
		contents := make([]string, len(groupDocs))
		removeIDs := make([]string, len(groupDocs))
		for i, doc := range groupDocs {
			contents[i] = doc.Content
			removeIDs[i] = doc.ID
		}
		mergedContent, usedStrategy := d.mergeFuseContents(ctx, contents, normalized)
		if strings.TrimSpace(mergedContent) == "" {
			continue
		}
		vec, embedErr := d.embedText(ctx, model, mergedContent)
		if embedErr != nil {
			logger.Warn("FileManager", "融合簇跳过（整合向量生成失败）: %v", embedErr)
			continue
		}
		role := opts.Role
		if strings.TrimSpace(role) == "" {
			role = majorityRole(groupDocs)
		}
		doc := Document{
			ID:        generateUUID(),
			Type:      MemoryTypeText,
			Role:      role,
			Content:   mergedContent,
			Source:    "fusion",
			Embedding: vec,
			Timestamp: time.Now().Unix(),
		}
		pending = append(pending, pendingMerge{
			removeIDs: removeIDs,
			doc:       doc,
			group: MemoryFuseGroup{
				IDs:        removeIDs,
				Similarity: cluster.minSimilarity,
				Content:    mergedContent,
				Role:       role,
				NewID:      doc.ID,
				Strategy:   usedStrategy,
			},
		})
	}

	if len(pending) == 0 {
		return result, nil
	}

	// 5. 一次性替换：删除原条目、追加整合条目，然后整体落盘
	remove := make(map[string]bool)
	for _, item := range pending {
		for _, id := range item.removeIDs {
			remove[id] = true
		}
	}
	c.mu.Lock()
	kept := make([]Document, 0, len(c.Documents))
	for _, doc := range c.Documents {
		if !remove[doc.ID] {
			kept = append(kept, doc)
		}
	}
	for _, item := range pending {
		kept = append(kept, item.doc)
		result.Groups = append(result.Groups, item.group)
	}
	c.Documents = kept
	c.mu.Unlock()

	if err := c.saveDocumentsToFile(); err != nil {
		return result, fmt.Errorf("保存融合结果失败: %w", err)
	}
	c.updateLastModTime()

	result.GroupCount = len(pending)
	result.CreatedCount = len(pending)
	result.RemovedCount = len(remove)
	logger.Info("FileManager", "集合 [%s] 权重融合完成：%d 组，删除 %d 条，新增 %d 条（阈值 %.2f）",
		collectionName, result.GroupCount, result.RemovedCount, result.CreatedCount, threshold)
	return result, nil
}

// NormalizeFuseStrategy 归一化合并策略
func NormalizeFuseStrategy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", MemoryFuseStrategyAuto:
		return MemoryFuseStrategyAuto, nil
	case MemoryFuseStrategyConcat:
		return MemoryFuseStrategyConcat, nil
	case MemoryFuseStrategyModel:
		return MemoryFuseStrategyModel, nil
	default:
		return "", fmt.Errorf("未知的融合策略: %s（可用 auto / concat / model）", value)
	}
}

// normalizeFuseThreshold 夹取相似度阈值：非法或过低时回落到默认值
func normalizeFuseThreshold(value float32) float32 {
	if value <= 0 || value < MemoryFuseMinThreshold || value > 1 {
		return MemoryFuseDefaultThreshold
	}
	return value
}

// fuseCluster 一个相似簇：indices 指向候选切片，首元素为代表条目
type fuseCluster struct {
	indices       []int
	minSimilarity float32
}

// clusterMemoryDocs 代表式聚类：每条与已有簇的代表条目比较，达阈值即并入最相似的簇
func clusterMemoryDocs(docs []Document, threshold float32) []fuseCluster {
	clusters := make([]fuseCluster, 0)
	for i, doc := range docs {
		bestIndex := -1
		bestSim := float32(0)
		for ci := range clusters {
			rep := docs[clusters[ci].indices[0]]
			sim := cosineSimilarity(doc.Embedding, rep.Embedding)
			if sim >= threshold && sim > bestSim {
				bestSim = sim
				bestIndex = ci
			}
		}
		if bestIndex >= 0 {
			clusters[bestIndex].indices = append(clusters[bestIndex].indices, i)
			if bestSim < clusters[bestIndex].minSimilarity {
				clusters[bestIndex].minSimilarity = bestSim
			}
			continue
		}
		clusters = append(clusters, fuseCluster{indices: []int{i}, minSimilarity: 1})
	}
	return clusters
}

// majorityRole 取簇内出现次数最多的角色；全为空时按用户消息处理
func majorityRole(docs []Document) string {
	counts := make(map[string]int)
	for _, doc := range docs {
		role := strings.TrimSpace(doc.Role)
		if role != "" {
			counts[role]++
		}
	}
	bestRole := ""
	bestCount := 0
	for role, count := range counts {
		if count > bestCount {
			bestRole = role
			bestCount = count
		}
	}
	if bestRole == "" {
		return "user"
	}
	return bestRole
}

// mergeFuseContents 按策略合并正文，返回合并结果与实际使用的方式
func (d *MemoryDB) mergeFuseContents(ctx context.Context, contents []string, strategy string) (string, string) {
	concatenated := concatMemoryContents(contents)
	if strategy == MemoryFuseStrategyConcat {
		return concatenated, MemoryFuseStrategyConcat
	}
	if strategy == MemoryFuseStrategyAuto && len([]rune(concatenated)) <= MemoryFuseAutoModelChars {
		return concatenated, MemoryFuseStrategyConcat
	}
	merged, err := d.mergeMemoryContentsWithModel(ctx, contents)
	if err != nil {
		logger.Warn("FileManager", "模型整合失败，回退为拼接: %v", err)
		return concatenated, MemoryFuseStrategyConcat
	}
	return merged, MemoryFuseStrategyModel
}

// concatMemoryContents 拼接去重：逐行去除完全重复内容，保留各条原始表述
func concatMemoryContents(contents []string) string {
	seen := make(map[string]bool)
	parts := make([]string, 0, len(contents))
	for _, content := range contents {
		kept := make([]string, 0, 4)
		for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			key := strings.Join(strings.Fields(trimmed), " ")
			if seen[key] {
				continue
			}
			seen[key] = true
			kept = append(kept, trimmed)
		}
		if len(kept) > 0 {
			parts = append(parts, strings.Join(kept, "\n"))
		}
	}
	return strings.Join(parts, "\n")
}

// mergeMemoryContentsWithModel 调用多模态/文本模型把多条记忆整合为一条完整记忆
func (d *MemoryDB) mergeMemoryContentsWithModel(ctx context.Context, contents []string) (string, error) {
	if strings.TrimSpace(config.AgentMultimodalURL) == "" {
		return "", fmt.Errorf("LLM 服务地址未配置")
	}
	clipped := make([]string, len(contents))
	for i, content := range contents {
		runes := []rune(content)
		if len(runes) > MemoryFuseMaxContentChars {
			runes = runes[:MemoryFuseMaxContentChars]
		}
		clipped[i] = fmt.Sprintf("%d. %s", i+1, string(runes))
	}

	const systemPrompt = "你是记忆整理助手。下面多条记忆在语义上高度相似，请把它们合并为一条完整、精炼、无重复的记忆。" +
		"要求：保留全部关键事实（人名、时间、地点、数字、偏好、结论），删除重复表述，不要添加原文没有的信息，" +
		"不要分点、不要标题、不要解释，只输出合并后的正文。"

	reqBody := chatRequest{
		Model: config.AgentMultimodalModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "待合并的记忆：\n" + strings.Join(clipped, "\n\n")},
		},
		MaxTokens:   1024,
		Temperature: 0.2,
		// 与标签生成一致：显式关闭思考，避免推理模型把 token 预算耗在 reasoning 上
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化整合请求失败: %w", err)
	}

	apiURL := strings.TrimRight(config.AgentMultimodalURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建整合请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if config.AgentMultimodalKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.AgentMultimodalKey)
	}

	client := d.httpClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用整合模型失败: %w", err)
	}
	defer resp.Body.Close()

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析整合响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if result.Error != nil && result.Error.Message != "" {
			return "", fmt.Errorf("整合模型返回错误: %s", result.Error.Message)
		}
		return "", fmt.Errorf("整合模型返回状态码: %d", resp.StatusCode)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("整合模型返回空结果")
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("整合模型返回空内容")
	}
	return content, nil
}

// MemoryFuseEntries 全局包装：对全局记忆库执行权重融合
func MemoryFuseEntries(ctx context.Context, collectionName string, ids []string, opts MemoryFuseOptions) (MemoryFuseResult, error) {
	if globalMemoryDB == nil {
		return MemoryFuseResult{}, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryFuseEntries(ctx, collectionName, ids, opts)
}
