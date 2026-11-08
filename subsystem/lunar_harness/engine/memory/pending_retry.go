package memory

// pending_retry.go 待处理条目的失败记账与放行兜底。
//
// 整理失败（模型不可用、校验不通过、落盘失败等）时会保留 Pending，等待下一轮重试。
// 但「下一轮再试」不能是唯一出口：同一批条目连续失败，或长期停留在 Pending，
// 必须有确定性的放行机制，否则新记忆会永久对检索隐身。
//
// 放行只解除原有条目的待处理标记，不写入任何半成品：
// 这些条目本身就是完整记忆，放行后只是重新参与检索，不存在部分合并的数据暴露。

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RecordPendingFailure 为一批待处理条目累计一次整理失败。
// 达到 maxAttempts 或隐身超过 maxAge 后解除待处理标记，让条目重新对检索可见；
// 其余条目继续保持待处理，等待下一轮。
func (d *Store) RecordPendingFailure(ctx context.Context, ids []string, maxAttempts int, maxAge time.Duration) (PendingFailureResult, error) {
	_ = ctx
	var result PendingFailureResult
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return result, nil
	}

	now := time.Now().Unix()
	var saveErr error
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.Lock()
		changed := false
		for i := range c.Documents {
			doc := &c.Documents[i]
			if _, ok := wanted[doc.ID]; !ok || !doc.Pending {
				continue
			}
			doc.PendingAttempts++
			if doc.PendingSince <= 0 {
				doc.PendingSince = now
			}
			expired := maxAge > 0 && now-doc.PendingSince >= int64(maxAge/time.Second)
			if doc.PendingAttempts >= maxAttempts || expired {
				doc.Pending = false
				doc.PendingPartner = ""
				doc.PendingAttempts = 0
				doc.PendingSince = 0
				result.Released = append(result.Released, doc.ID)
			} else {
				result.Retained = append(result.Retained, doc.ID)
			}
			changed = true
		}
		c.mu.Unlock()
		if !changed {
			continue
		}
		if err := c.saveDocumentsToFile(); err != nil {
			saveErr = errors.Join(saveErr, err)
			continue
		}
		for _, id := range result.Released {
			d.keywordIndexSetPending(id, false)
		}
		c.updateLastModTime()
	}
	return result, saveErr
}
