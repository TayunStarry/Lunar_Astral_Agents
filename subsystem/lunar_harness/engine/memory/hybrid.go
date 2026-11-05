package memory

// hybrid.go 混合检索：向量通道与关键词通道并行召回，按融合分数排序。
//
// 设计要点：
//   - 两路各自召回候选，同一记忆 ID 只保留一条，避免重复返回；
//   - 向量分按 DocRankThreshold 线性归一化到 0~1，与关键词分量纲对齐；
//   - 关键词分由「倒排索引加权覆盖率」构成，天然落在 0~1（常见词按 df 降权，精确串加权）；
//   - 融合分 = 归一化向量分 × 向量权重 + 关键词强度 × 关键词权重；
//   - 排序键：融合分降序 → 向量分降序 → 入库时间降序 → ID 升序（保证结果可复现）；
//   - 嵌入服务不可用或超时时，向量通道失败但关键词通道照常返回，整条检索不失败；
//   - 共享 topK 预算：两通道共用一个返回额度，不固定对半。先按融合分做全局排序，
//     某一路召回不足时，另一路的高分候选自然补足空余额度，总数严格不超过 topK。

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// HybridQuery 混合检索记忆：向量检索与关键词检索同时执行，
// 候选按记忆 ID 去重、分数归一化后融合，并按综合相关度排序返回。
//
// queries 为调用方给出的关键词/短句列表（通常 1~4 个）；topK 为最终返回条数。
// 任一通道失败时，只要另一通道有结果就正常返回；只有两路均失败且无结果时才返回错误。
func (d *Store) HybridQuery(ctx context.Context, queries []string, mediaType string, topK int, opts HybridQueryOptions) ([]Entry, error) {
	if topK <= 0 {
		return nil, nil
	}
	queries = normalizeHybridQueries(queries)
	if len(queries) == 0 {
		return nil, nil
	}
	normalizedType, err := NormalizeType(mediaType)
	if err != nil {
		return nil, err
	}

	// 共享 topK 预算：两通道共用同一个召回上限（不低于 topK），
	// 保证任何一路富余时都有足够的候选去补足另一路的空余额度。
	recallLimit := hybridRecallLimit(topK, opts.CandidateLimit)
	vectorWeight, keywordWeight := hybridWeights(opts)

	// 两路并行前先统一刷新一次集合，避免两个通道同时触发跨进程重载。
	for _, collection := range d.snapshotCollections() {
		collection.reloadIfChanged()
	}

	var (
		vectorHits  []Entry
		keywordHits []Entry
		vectorErr   error
		keywordErr  error
		wg          sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		vectorHits, vectorErr = d.hybridVectorChannel(ctx, queries, normalizedType, recallLimit, opts.ForceGlobal)
	}()
	go func() {
		defer wg.Done()
		keywordHits, keywordErr = d.hybridKeywordChannel(queries, normalizedType, recallLimit, opts.Regex)
	}()
	wg.Wait()

	if vectorErr != nil && keywordErr != nil && len(vectorHits) == 0 && len(keywordHits) == 0 {
		return nil, fmt.Errorf("记忆混合检索失败：向量通道 %v；关键词通道 %v", vectorErr, keywordErr)
	}
	if vectorErr != nil {
		// 嵌入服务不可用/超时：降级为关键词结果，不影响调用方
		logger.Warn("FileManager", "记忆混合检索：向量通道不可用，已降级为关键词结果（%v）", vectorErr)
	}
	if keywordErr != nil {
		logger.Warn("FileManager", "记忆混合检索：关键词通道失败，仅返回向量结果（%v）", keywordErr)
	}

	fused := fuseHybridCandidates(vectorHits, keywordHits, vectorWeight, keywordWeight)
	return trimHybridBudget(fused, topK), nil
}

// trimHybridBudget 共享 topK 预算的裁剪入口。
//
// 融合结果已按综合分全局排序，因此额度按“分数优先”自动调配：
//   - 关键词结果不足（例如只召回 3 条）时，向量的高分候选继续占据剩余名额；
//   - 向量结果不足时同理由关键词补足；
//   - 两路均富余时按分数择优，不做固定 0.6/0.4 的名额切分。
//
// 唯一硬约束是总返回条数不超过 topK；降级（仅剩一路）时同样走这里。
func trimHybridBudget(fused []Entry, topK int) []Entry {
	if topK <= 0 || len(fused) == 0 {
		return nil
	}
	if len(fused) > topK {
		return fused[:topK]
	}
	return fused
}

// hybridBudgetUsage 统计返回结果中各通道实际占用的额度（用于测试与观测）。
func hybridBudgetUsage(hits []Entry) (vectorOnly, keywordOnly, both int) {
	for _, hit := range hits {
		switch hit.MatchedBy {
		case MatchedByBoth:
			both++
		case MatchedByVector:
			vectorOnly++
		case MatchedByKeyword:
			keywordOnly++
		}
	}
	return vectorOnly, keywordOnly, both
}

// hybridVectorChannel 向量通道：逐条查询做嵌入语义检索，按 ID 合并取最高相似度。
// 嵌入调用受 HybridVectorTimeout 约束，服务不可用时尽快失败以让关键词通道返回。
func (d *Store) hybridVectorChannel(ctx context.Context, queries []string, mediaType string, limit int, forceGlobal bool) ([]Entry, error) {
	vectorCtx, cancel := context.WithTimeout(ctx, HybridVectorTimeout)
	defer cancel()

	merged := make(map[string]Entry, len(queries)*limit)
	var lastErr error
	for _, query := range queries {
		results, err := d.Query(vectorCtx, query, mediaType, limit, QueryOptions{ForceGlobal: forceGlobal})
		if err != nil {
			lastErr = err
			continue
		}
		for _, item := range results {
			if item.ID == "" {
				continue
			}
			existing, ok := merged[item.ID]
			if !ok || item.Similarity > existing.Similarity {
				merged[item.ID] = item
			}
		}
	}
	if len(merged) == 0 && lastErr != nil {
		return nil, lastErr
	}

	out := make([]Entry, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Similarity > out[j].Similarity
	})
	return out, nil
}

// hybridKeywordChannel 关键词通道：默认走倒排索引独立召回，按加权覆盖率打分；
// 正则模式保留原有的全库字面扫描（正则无法安全地映射为词元召回）。
// 不依赖嵌入服务，因此向量通道不可用时仍可独立返回结果。
func (d *Store) hybridKeywordChannel(queries []string, mediaType string, limit int, useRegex bool) ([]Entry, error) {
	if !useRegex {
		hits, _ := d.keywordIndexRecall(queries, mediaType, limit)
		return hits, nil
	}

	score, err := hybridKeywordScorer(queries, true)
	if err != nil {
		return nil, err
	}
	if score == nil {
		return nil, nil
	}

	merged := make(map[string]Entry)
	for _, collection := range d.snapshotCollections() {
		collection.mu.RLock()
		for i := range collection.Documents {
			doc := &collection.Documents[i]
			if documentType(*doc) != mediaType || strings.TrimSpace(doc.Content) == "" {
				continue
			}
			// 待处理条目只对整理可见：检索默认不返回，避免同一事实出现两遍
			if doc.Pending {
				continue
			}
			keywordScore := score(doc.Content)
			if keywordScore <= 0 {
				continue
			}
			if existing, ok := merged[doc.ID]; ok && existing.KeywordScore >= keywordScore {
				continue
			}
			entry := resultOf(collection, doc, 0)
			entry.KeywordScore = keywordScore
			merged[doc.ID] = entry
		}
		collection.mu.RUnlock()
	}

	out := make([]Entry, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].KeywordScore != out[j].KeywordScore {
			return out[i].KeywordScore > out[j].KeywordScore
		}
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp > out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// hybridKeywordScorer 构造关键词打分函数；返回 nil 表示没有可用的关键词。
// 普通模式：覆盖度 × 命中次数强度；正则模式：命中次数强度（覆盖度视为 1）。
func hybridKeywordScorer(queries []string, useRegex bool) (func(string) float32, error) {
	if useRegex {
		pattern := strings.Join(queries, " ")
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("正则表达式无效: %w", err)
		}
		return func(text string) float32 {
			occurrences := len(re.FindAllStringIndex(text, HybridKeywordOccurrenceCap))
			return hybridKeywordOccurrenceFactor(occurrences)
		}, nil
	}

	terms := hybridTerms(queries)
	if len(terms) == 0 {
		return nil, nil
	}
	return func(text string) float32 {
		matched := 0
		occurrences := 0
		for _, term := range terms {
			count := countTermOccurrences(text, term)
			if count > 0 {
				matched++
				occurrences += count
			}
		}
		if matched == 0 {
			return 0
		}
		coverage := float32(matched) / float32(len(terms))
		return coverage * hybridKeywordOccurrenceFactor(occurrences)
	}, nil
}

// hybridKeywordOccurrenceFactor 把命中次数映射到 [HybridKeywordOccurrenceFloor, 1]：
// 单次命中为下限，命中次数达到上限时接近 1。
func hybridKeywordOccurrenceFactor(occurrences int) float32 {
	if occurrences <= 0 {
		return 0
	}
	if occurrences > HybridKeywordOccurrenceCap {
		occurrences = HybridKeywordOccurrenceCap
	}
	ratio := float32(occurrences) / float32(HybridKeywordOccurrenceCap)
	return HybridKeywordOccurrenceFloor + (1-HybridKeywordOccurrenceFloor)*ratio
}

// countTermOccurrences 统计不区分大小写的字面命中次数（最多计到上限，避免长文本空转）。
func countTermOccurrences(text, term string) int {
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(strings.TrimSpace(term))
	if lowerTerm == "" {
		return 0
	}
	count := 0
	offset := 0
	for {
		index := strings.Index(lowerText[offset:], lowerTerm)
		if index < 0 {
			break
		}
		count++
		offset += index + len(lowerTerm)
		if count >= HybridKeywordOccurrenceCap {
			break
		}
	}
	return count
}

// hybridTerms 从查询短句中提取关键词项：短句整体作为一个词，长句按空白切分。
func hybridTerms(queries []string) []string {
	terms := make([]string, 0, len(queries)*2)
	seen := make(map[string]bool, len(queries)*2)
	appendTerm := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if utf8.RuneCountInString(query) <= 20 {
			appendTerm(query)
			continue
		}
		for _, part := range strings.Fields(query) {
			if utf8.RuneCountInString(part) >= 2 {
				appendTerm(part)
			}
		}
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

// fuseHybridCandidates 合并两路候选：按 ID 去重、分量归一化、加权融合并排序。
func fuseHybridCandidates(vectorHits, keywordHits []Entry, vectorWeight, keywordWeight float32) []Entry {
	candidates := make(map[string]*hybridCandidate, len(vectorHits)+len(keywordHits))

	for _, hit := range vectorHits {
		if hit.ID == "" {
			continue
		}
		candidate := ensureHybridCandidate(candidates, hit)
		if score := normalizeVectorScore(hit.Similarity); score > candidate.vectorScore {
			candidate.vectorScore = score
		}
		candidate.vectorHit = true
	}
	for _, hit := range keywordHits {
		if hit.ID == "" {
			continue
		}
		candidate := ensureHybridCandidate(candidates, hit)
		if hit.KeywordScore > candidate.keywordScore {
			candidate.keywordScore = hit.KeywordScore
		}
		candidate.keywordHit = true
		if utf8.RuneCountInString(hit.Content) > utf8.RuneCountInString(candidate.entry.Content) {
			candidate.entry.Content = hit.Content
		}
	}

	out := make([]Entry, 0, len(candidates))
	for _, candidate := range candidates {
		entry := candidate.entry
		entry.VectorScore = candidate.vectorScore
		entry.KeywordScore = candidate.keywordScore
		entry.Similarity = vectorWeight*candidate.vectorScore + keywordWeight*candidate.keywordScore
		switch {
		case candidate.vectorHit && candidate.keywordHit:
			entry.MatchedBy = MatchedByBoth
		case candidate.vectorHit:
			entry.MatchedBy = MatchedByVector
		default:
			entry.MatchedBy = MatchedByKeyword
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Similarity != out[j].Similarity {
			return out[i].Similarity > out[j].Similarity
		}
		if out[i].VectorScore != out[j].VectorScore {
			return out[i].VectorScore > out[j].VectorScore
		}
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp > out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ensureHybridCandidate 获取或创建候选；同一 ID 只保留一条，正文取更完整的一份。
func ensureHybridCandidate(candidates map[string]*hybridCandidate, hit Entry) *hybridCandidate {
	if candidate, ok := candidates[hit.ID]; ok {
		if utf8.RuneCountInString(hit.Content) > utf8.RuneCountInString(candidate.entry.Content) {
			candidate.entry.Content = hit.Content
		}
		return candidate
	}
	candidate := &hybridCandidate{entry: hit}
	candidate.entry.Similarity = 0
	candidates[hit.ID] = candidate
	return candidate
}

// normalizeVectorScore 把余弦相似度按召回门槛线性映射到 0~1：
// 门槛处为 0，完全相似为 1，保证与关键词分量纲一致。
func normalizeVectorScore(similarity float32) float32 {
	if similarity <= DocRankThreshold {
		return 0
	}
	if similarity >= 1 {
		return 1
	}
	return (similarity - DocRankThreshold) / (1 - DocRankThreshold)
}

// hybridRecallLimit 计算共享召回上限：两通道使用同一个上限，且不低于 topK，
// 这样任一通道都能独立补齐另一通道的空余额度。
func hybridRecallLimit(topK, requested int) int {
	limit := hybridCandidateLimit(topK, requested)
	if limit < topK {
		limit = topK
	}
	return limit
}

// hybridCandidateLimit 计算召回上限：优先用调用方指定值，否则 topK×3，夹在 20~60。
func hybridCandidateLimit(topK, requested int) int {
	limit := requested
	if limit <= 0 {
		limit = topK * HybridCandidateMultiplier
	}
	if limit < HybridMinCandidates {
		limit = HybridMinCandidates
	}
	if limit > HybridMaxCandidates {
		limit = HybridMaxCandidates
	}
	return limit
}

// hybridWeights 归一化两路权重：均未配置时用默认值；只配置一路时另一路按 0 处理。
func hybridWeights(opts HybridQueryOptions) (float32, float32) {
	vectorWeight, keywordWeight := opts.VectorWeight, opts.KeywordWeight
	if vectorWeight <= 0 && keywordWeight <= 0 {
		vectorWeight, keywordWeight = HybridVectorWeight, HybridKeywordWeight
	}
	sum := vectorWeight + keywordWeight
	if sum <= 0 {
		return HybridVectorWeight, HybridKeywordWeight
	}
	return vectorWeight / sum, keywordWeight / sum
}

// normalizeHybridQueries 去除空白项并按原顺序去重（保持原大小写，避免影响嵌入语义）。
func normalizeHybridQueries(queries []string) []string {
	out := make([]string, 0, len(queries))
	seen := make(map[string]bool, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" || seen[query] {
			continue
		}
		seen[query] = true
		out = append(out, query)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}
