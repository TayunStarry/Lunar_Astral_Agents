package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// replace_audit.go 记忆整理事务的快照、指纹校验与回滚。

// snapshotTextDocs 按 ID 快照文本条目（含嵌入向量副本），非文本条目一律不纳入删除范围。
func (d *Store) snapshotTextDocs(ids map[string]struct{}) []docSnapshot {
	if len(ids) == 0 {
		return nil
	}
	var out []docSnapshot
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for _, doc := range c.Documents {
			if _, ok := ids[doc.ID]; !ok || documentType(doc) != TypeText {
				continue
			}
			cp := doc
			cp.Embedding = append([]float32(nil), doc.Embedding...)
			cp.ContentEmbedding = append([]float32(nil), doc.ContentEmbedding...)
			out = append(out, docSnapshot{Doc: cp, Collection: c.Name})
		}
		c.mu.RUnlock()
	}
	return out
}

// findExactTextDoc 查找与 content 完全同文的文本条目；exclude 中的 ID（待删除的旧条目）不参与复用。
func (d *Store) findExactTextDoc(content string, exclude map[string]struct{}) (string, bool) {
	want := normalizeTextKey(content)
	if want == "" {
		return "", false
	}
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for _, doc := range c.Documents {
			if documentType(doc) != TypeText || strings.TrimSpace(doc.Content) == "" {
				continue
			}
			if _, skip := exclude[doc.ID]; skip {
				continue
			}
			if normalizeTextKey(doc.Content) == want {
				id := doc.ID
				c.mu.RUnlock()
				return id, true
			}
		}
		c.mu.RUnlock()
	}
	return "", false
}

// verifyReplacementTexts 校验每条新内容都已存在于库中（排除待删除的旧条目）。
func (d *Store) verifyReplacementTexts(texts []string, oldSet map[string]struct{}) error {
	for _, text := range texts {
		if _, ok := d.findExactTextDoc(text, oldSet); !ok {
			return fmt.Errorf("内容未落库: %s", truncateForError(text))
		}
	}
	return nil
}

// verifyRemoved 校验快照中的旧条目全部已从库中消失。
func (d *Store) verifyRemoved(snapshots []docSnapshot) error {
	for _, snap := range snapshots {
		if _, err := d.Get(snap.Doc.ID); err == nil {
			return fmt.Errorf("旧记忆 %s 仍存在", snap.Doc.ID)
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// rollbackReplace 回滚一次失败的替换：先按快照恢复旧条目，再删除本次新建的条目。
// 回滚本身尽力而为，所有错误聚合返回，便于上层日志定位。
func (d *Store) rollbackReplace(ctx context.Context, createdIDs []string, snapshots []docSnapshot) error {
	var errs []error
	for _, snap := range snapshots {
		if _, err := d.Get(snap.Doc.ID); err == nil {
			continue // 尚未删除，无需恢复
		} else if !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Errorf("检查旧记忆 %s 失败: %w", snap.Doc.ID, err))
			continue
		}
		if err := d.restoreTextDoc(ctx, snap); err != nil {
			errs = append(errs, fmt.Errorf("恢复旧记忆 %s 失败: %w", snap.Doc.ID, err))
		}
	}
	for _, id := range createdIDs {
		if err := d.Delete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Errorf("删除新建记忆 %s 失败: %w", id, err))
		}
	}
	if len(errs) > 0 {
		logger.Warn("FileManager", "记忆整理回滚存在失败项: %v", errors.Join(errs...))
		return errors.Join(errs...)
	}
	logger.Info("FileManager", "记忆整理回滚完成：恢复旧 %d 条、清理新建 %d 条", len(snapshots), len(createdIDs))
	return nil
}

// restoreTextDoc 按快照恢复一条旧文本记忆（保留原 ID/角色/来源/入库时间）。
// 嵌入服务不可用时仍会落库正文（向量留空），优先保证内容不丢。
func (d *Store) restoreTextDoc(ctx context.Context, snap docSnapshot) error {
	doc := snap.Doc
	if documentType(doc) != TypeText {
		doc.Type = TypeText
	}
	var vec []float32
	if strings.TrimSpace(doc.Content) != "" {
		if got, err := d.embedText(ctx, currentEmbeddingModel(), doc.Content); err == nil {
			vec = got
		}
	}

	clusterID := snap.Collection
	switch {
	case len(vec) > 0:
		doc.Embedding = vec
		// 恢复时从正文重建向量：此时检索向量就是正文向量，无需再单独保存
		doc.ContentEmbedding = nil
		// 原簇可能已因条目删空而消失，按当前向量重新写入总库，保证目录与索引一致
		if err := d.assignToStore(doc.ID, vec); err == nil {
			clusterID = StoreCollectionID
		}
	case clusterID == "" || clusterID == StoreCollectionID:
		clusterID = StoreCollectionID
	}

	target, err := d.collectionForID(clusterID)
	if err != nil {
		target, err = d.storeCollection()
		if err != nil {
			return fmt.Errorf("没有可用的存储目录，无法恢复条目 %s: %w", doc.ID, err)
		}
	}
	if len(vec) == 0 && target.Name == snap.Collection {
		doc.Embedding = append([]float32(nil), snap.Doc.Embedding...)
		doc.ContentEmbedding = append([]float32(nil), snap.Doc.ContentEmbedding...)
	}
	if err := target.appendDocument(doc); err != nil {
		return err
	}
	d.keywordIndexAdd(target, doc)
	return nil
}

// truncateForError 仅用于错误信息，避免超长正文刷屏（512 字符起步，不丢实际数据）。
func truncateForError(text string) string {
	const limit = 512
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
