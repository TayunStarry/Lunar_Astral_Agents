package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// replace.go 记忆整理的批量替换事务。
//
// 与普通写入不同，整理会用少量新条目替换一批旧条目，必须满足两条硬约束：
//  1. 先写新、再删旧：新条目全部落库并校验通过之前，一条旧条目都不删；
//  2. 任一步失败立即回滚：删除本次新建的条目，并按快照恢复旧条目。
//
// 空结果（newTexts 为空或全为空白）一律拒绝，绝不把整批旧记忆清空。
func (d *Store) ReplaceTexts(ctx context.Context, oldIDs, newTexts []string, opts ReplaceOptions) (ReplaceResult, error) {
	var result ReplaceResult
	if d == nil {
		return result, fmt.Errorf("记忆库未初始化")
	}
	role := strings.TrimSpace(opts.Role)
	if role == "" {
		role = "assistant"
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "consolidation"
	}

	texts := normalizeReplacementTexts(newTexts)
	if len(texts) == 0 {
		return result, fmt.Errorf("整理结果为空：拒绝删除任何旧记忆")
	}

	oldSet := make(map[string]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		if id = strings.TrimSpace(id); id != "" {
			oldSet[id] = struct{}{}
		}
	}

	// 1. 删除前完整快照（含角色/来源/入库时间/向量与所属集合），供失败回滚使用
	snapshots := d.snapshotTextDocs(oldSet)
	if len(oldSet) > 0 && len(snapshots) == 0 {
		logger.Warn("FileManager", "记忆整理：待替换的 %d 条旧记忆均已不存在，仅写入新条目", len(oldSet))
	}

	// 2. 先写新条目；完全同文的既有条目直接复用，避免重复写入与恢复重放产生副本。
	// 复用到的条目如果还在待处理队列里，这里同步清掉标记：整理结果已经落库，
	// 不能再让同一条记忆继续对检索隐藏。
	for _, text := range texts {
		if id, ok := d.findExactTextDoc(text, oldSet); ok {
			if err := d.ClearPending(ctx, id); err != nil {
				logger.Warn("FileManager", "记忆整理：清除复用条目 %s 的待处理标记失败: %v", id, err)
			}
			result.ReusedIDs = append(result.ReusedIDs, id)
			continue
		}
		id, err := d.addTextNoDedup(ctx, role, text, source)
		if err != nil {
			rbErr := d.rollbackReplace(ctx, result.CreatedIDs, snapshots)
			return result, fmt.Errorf("写入整理后记忆失败（旧记忆已回滚）: %w", errors.Join(err, rbErr))
		}
		result.CreatedIDs = append(result.CreatedIDs, id)
	}

	// 3. 数量校验：新建 + 复用的条目数必须与新内容条数一致
	if got := len(result.CreatedIDs) + len(result.ReusedIDs); got != len(texts) {
		rbErr := d.rollbackReplace(ctx, result.CreatedIDs, snapshots)
		msg := fmt.Sprintf("整理后条目数量校验失败（期望 %d，实际 %d）", len(texts), got)
		if rbErr != nil {
			return result, fmt.Errorf("%s，且回滚异常: %w", msg, rbErr)
		}
		return result, errors.New(msg + "，已回滚")
	}
	// 4. 内容指纹校验：每条新内容都必须能在库中找到（排除待删除的旧条目）
	if err := d.verifyReplacementTexts(texts, oldSet); err != nil {
		rbErr := d.rollbackReplace(ctx, result.CreatedIDs, snapshots)
		return result, fmt.Errorf("整理后内容校验失败（旧记忆已回滚）: %w", errors.Join(err, rbErr))
	}

	// 5. 新内容确认无误后才删除旧条目
	for _, snap := range snapshots {
		err := d.Delete(ctx, snap.Doc.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			rbErr := d.rollbackReplace(ctx, result.CreatedIDs, snapshots)
			return result, fmt.Errorf("删除旧记忆 %s 失败（已回滚）: %w", snap.Doc.ID, errors.Join(err, rbErr))
		}
		result.DeletedIDs = append(result.DeletedIDs, snap.Doc.ID)
	}

	// 5. 数量/指纹复核：旧条目必须全部消失
	if err := d.verifyRemoved(snapshots); err != nil {
		rbErr := d.rollbackReplace(ctx, result.CreatedIDs, snapshots)
		return result, fmt.Errorf("旧记忆删除校验失败（已回滚）: %w", errors.Join(err, rbErr))
	}
	return result, nil
}

// normalizeReplacementTexts 去空白、去完全重复的正文，保留原始顺序。
func normalizeReplacementTexts(texts []string) []string {
	out := make([]string, 0, len(texts))
	seen := make(map[string]struct{}, len(texts))
	for _, raw := range texts {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		key := normalizeTextKey(text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, text)
	}
	return out
}

// normalizeTextKey 内容指纹：压缩连续空白并去首尾空格，用于「完全同文」判定。
func normalizeTextKey(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

// addTextNoDedup 无条件写入文本（跳过向量相似度判重，避免整理结果被旧条目判定为重复而丢弃）。
func (d *Store) addTextNoDedup(ctx context.Context, role, content, source string) (string, error) {
	vec, err := d.embedText(ctx, currentEmbeddingModel(), content)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}
	id := generateUUID()
	if err := d.assignToStore(id, vec); err != nil {
		return "", fmt.Errorf("写入总库失败: %w", err)
	}
	c, err := d.storeCollection()
	if err != nil {
		d.detachCluster(id)
		return "", err
	}
	doc := Document{
		ID:        id,
		Type:      TypeText,
		Role:      role,
		Content:   content,
		Source:    source,
		Embedding: vec,
		Timestamp: time.Now().Unix(),
	}
	if err := c.appendDocument(doc); err != nil {
		d.detachCluster(id)
		return "", err
	}
	d.keywordIndexAdd(c, doc)
	return id, nil
}
