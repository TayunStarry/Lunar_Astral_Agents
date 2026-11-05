package memory

// keyword_recall.go 关键词召回

import (
	"math"
	"sort"
)

func documentIDs(docs []Document) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc.ID != "" {
			ids = append(ids, doc.ID)
		}
	}
	return ids
}

// ===== 召回与打分 =====

// keywordQueryTokens 从查询串提取参与召回的词元：过滤不在词表中的词元，
// 超限时优先保留精确串与稀有词，保证精确串可单独触发召回。
func (idx *keywordIndex) keywordQueryTokens(queries []string) []string {
	seen := make(map[string]struct{})
	var tokens []string
	for _, q := range queries {
		for token := range keywordTokenize(q) {
			if _, ok := idx.tokens[token]; !ok {
				continue
			}
			if _, dup := seen[token]; dup {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
		}
	}
	if len(tokens) > KeywordMaxQueryTokens {
		sort.Slice(tokens, func(i, j int) bool {
			ei, ej := keywordTokenExact(tokens[i]), keywordTokenExact(tokens[j])
			if ei != ej {
				return ei
			}
			di, dj := len(idx.tokens[tokens[i]]), len(idx.tokens[tokens[j]])
			if di != dj {
				return di < dj
			}
			return tokens[i] < tokens[j]
		})
		tokens = tokens[:KeywordMaxQueryTokens]
	}
	return tokens
}

func keywordTokenWeight(df int, exact bool) float32 {
	if df <= 0 {
		return 0
	}
	w := float32(1) / float32(1+math.Log2(float64(df)+1))
	if exact {
		w *= KeywordExactWeight
	}
	return w
}

// recall 倒排召回并按加权覆盖率打分。
// 覆盖率 = 命中词元权重和 / 全部生效查询词元权重和；词频不参与打分，常见词按 df 降权。
func (idx *keywordIndex) recall(queries []string, mediaType string) ([]keywordHitCandidate, keywordRecallStats) {
	stats := keywordRecallStats{UsedIndex: true}
	tokens := idx.keywordQueryTokens(queries)
	stats.QueryTokens = len(tokens)
	if len(tokens) == 0 {
		return nil, stats
	}

	var denominator float32
	for _, token := range tokens {
		denominator += keywordTokenWeight(len(idx.tokens[token]), keywordTokenExact(token))
	}
	if denominator <= 0 {
		return nil, stats
	}

	scores := make(map[uint32]float32)
	for _, token := range tokens {
		weight := keywordTokenWeight(len(idx.tokens[token]), keywordTokenExact(token))
		for _, posting := range idx.tokens[token] {
			stats.PostingsScanned++
			kd := idx.ords[posting.Ord]
			if kd == nil || kd.Pending {
				continue
			}
			if mediaType != "" && kd.Type != mediaType {
				continue
			}
			scores[posting.Ord] += weight
			stats.DocsScored++
		}
	}
	stats.Candidates = len(scores)

	candidates := make([]keywordHitCandidate, 0, len(scores))
	for ord, matched := range scores {
		kd := idx.ords[ord]
		if kd == nil {
			continue
		}
		score := matched / denominator
		if score < KeywordMinScore {
			continue
		}
		candidates = append(candidates, keywordHitCandidate{
			ID:         kd.ID,
			Collection: kd.Collection,
			Timestamp:  kd.Timestamp,
			Score:      score,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Timestamp != candidates[j].Timestamp {
			return candidates[i].Timestamp > candidates[j].Timestamp
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates, stats
}

// keywordIndexRecall 执行索引召回并构建 Entry（最多 limit 条）。
func (d *Store) keywordIndexRecall(queries []string, mediaType string, limit int) ([]Entry, keywordRecallStats) {
	d.ensureKeywordIndex()
	d.keywordMu.RLock()
	idx := d.keywordIndex
	if idx == nil {
		d.keywordMu.RUnlock()
		return nil, keywordRecallStats{}
	}
	candidates, stats := idx.recall(queries, mediaType)
	d.keywordMu.RUnlock()

	if limit <= 0 {
		limit = 1
	}
	out := make([]Entry, 0, limit)
	for _, cand := range candidates {
		if len(out) >= limit {
			break
		}
		c := d.collectionByName(cand.Collection)
		if c == nil {
			continue
		}
		doc, ok := c.documentByID(cand.ID)
		if !ok || doc.Pending {
			continue
		}
		if mediaType != "" && documentType(doc) != mediaType {
			continue
		}
		entry := resultOf(c, &doc, 0)
		entry.KeywordScore = cand.Score
		out = append(out, entry)
	}
	stats.Hits = len(out)
	return out, stats
}
