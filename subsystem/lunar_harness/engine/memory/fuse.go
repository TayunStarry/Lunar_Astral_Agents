package memory

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// =============================================================================
// 权重融合 — 把一个集合内「超高相似度」的文本记忆合并成一条整合记忆
//
// 仅处理文本记忆：图片/音频记忆没有可比较的正文，也不应被合并（其向量来自标签/描述）。
// 流程：取候选 → 补齐内容向量 → 以余弦相似度做代表式聚类 → 按策略合并正文
//       → 用合并正文重新生成嵌入向量 → 删除原条目并写入整合条目。
// =============================================================================

// Fuse 对指定集合的文本记忆执行权重融合（管理端点使用的显式操作，不参与自动整理调度）。
//
// ids 为空表示扫描整个集合；ids 非空时只处理这些条目。
// 与 chain/life 的生活记忆整理共用同一套聚类原语、嵌入校验与 ReplaceTexts 事务，
// 不再自己删除/追加文档，避免出现第二条绕过校验与回滚的写入路径。
func (d *Store) Fuse(ctx context.Context, collectionName string, ids []string, opts FuseOptions) (FuseResult, error) {
	result := FuseResult{Collection: collectionName, Groups: []FuseGroup{}}
	normalized, err := NormalizeFuseStrategy(opts.Strategy)
	if err != nil {
		return result, err
	}
	threshold := normalizeFuseThreshold(opts.Threshold)
	result.Threshold = threshold

	// 1. 取候选 + 代表式聚类（与生活整理共用 cluster.go 的实现）
	clusters, candidates, skipped, err := d.clusterTextCandidates(ctx, collectionName, ids, threshold, FuseMaxCandidates)
	if err != nil {
		return result, err
	}
	result.CandidateCount = candidates
	result.SkippedMedia = skipped
	if len(clusters) == 0 {
		return result, nil
	}

	// 2. 逐簇合并：模型/嵌入阶段不持锁，写回统一走 ReplaceTexts
	for _, cluster := range clusters {
		mergedContent, usedStrategy := d.mergeFuseContents(ctx, cluster.Contents, normalized)
		if strings.TrimSpace(mergedContent) == "" {
			continue
		}
		if verifyErr := d.VerifyReplacementSimilarity(ctx, cluster.IDs, []string{mergedContent}, ClusterVerifyThreshold); verifyErr != nil {
			logger.Warn("FileManager", "融合簇跳过（向量校验未通过）: %v", verifyErr)
			continue
		}
		role := opts.Role
		if strings.TrimSpace(role) == "" {
			role = cluster.Role
		}
		replaced, replaceErr := d.ReplaceTexts(ctx, cluster.IDs, []string{mergedContent}, ReplaceOptions{
			Role:   role,
			Source: fuseReplaceSource,
		})
		if replaceErr != nil {
			logger.Warn("FileManager", "融合簇替换失败，原条目保留: %v", replaceErr)
			continue
		}
		newID := ""
		if len(replaced.CreatedIDs) > 0 {
			newID = replaced.CreatedIDs[0]
		} else if len(replaced.ReusedIDs) > 0 {
			newID = replaced.ReusedIDs[0]
		}
		result.Groups = append(result.Groups, FuseGroup{
			IDs:        cluster.IDs,
			Similarity: cluster.MinSimilarity,
			Content:    mergedContent,
			Role:       role,
			NewID:      newID,
			Strategy:   usedStrategy,
		})
		result.GroupCount++
		result.CreatedCount += len(replaced.CreatedIDs)
		result.RemovedCount += len(replaced.DeletedIDs)
	}

	if result.GroupCount == 0 {
		return result, nil
	}
	logger.Info("FileManager", "集合 [%s] 权重融合完成：%d 组，删除 %d 条，新增 %d 条（阈值 %.2f）",
		collectionName, result.GroupCount, result.RemovedCount, result.CreatedCount, threshold)
	return result, nil
}

// NormalizeFuseStrategy 归一化合并策略
func NormalizeFuseStrategy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", FuseStrategyAuto:
		return FuseStrategyAuto, nil
	case FuseStrategyConcat:
		return FuseStrategyConcat, nil
	case FuseStrategyModel:
		return FuseStrategyModel, nil
	default:
		return "", fmt.Errorf("未知的融合策略: %s（可用 auto / concat / model）", value)
	}
}

// normalizeFuseThreshold 夹取相似度阈值：非法或过低时回落到默认值
func normalizeFuseThreshold(value float32) float32 {
	if value <= 0 || value < FuseMinThreshold || value > 1 {
		return FuseDefaultThreshold
	}
	return value
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
func (d *Store) mergeFuseContents(ctx context.Context, contents []string, strategy string) (string, string) {
	concatenated := concatMemoryContents(contents)
	if strategy == FuseStrategyConcat {
		return concatenated, FuseStrategyConcat
	}
	if strategy == FuseStrategyAuto && len([]rune(concatenated)) <= FuseAutoModelChars {
		return concatenated, FuseStrategyConcat
	}
	merged, err := d.mergeMemoryContentsWithModel(ctx, contents)
	if err != nil {
		logger.Warn("FileManager", "模型整合失败，回退为拼接: %v", err)
		return concatenated, FuseStrategyConcat
	}
	return merged, FuseStrategyModel
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

// mergeMemoryContentsWithModel 调用记忆融合智能体把多条记忆整合为一条完整记忆
func (d *Store) mergeMemoryContentsWithModel(ctx context.Context, contents []string) (string, error) {
	if MemoryMergeFunc == nil {
		return "", fmt.Errorf("记忆融合智能体未装配")
	}
	return MemoryMergeFunc(ctx, contents)
}
