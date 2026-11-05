package life

// memory_archive.go 记忆整理淘汰归档：被替换掉的旧条目先追加到归档日志，再执行删除。
//
// 归档失败时调用方会跳过本批替换，保证「淘汰必有归档可追溯」。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// memoryArchiveRecord 单条被淘汰记忆的归档记录。
type memoryArchiveRecord struct {
	BatchID    string `json:"batch_id"`
	ID         string `json:"id"`
	Content    string `json:"content"`
	Role       string `json:"role,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	Reason     string `json:"reason"`
	ArchivedAt int64  `json:"archived_at"`
}

// archiveMemoryBatch 把一批即将被替换的旧条目追加到归档日志（JSON Lines）。
// 追加成功才返回 nil；任何失败都会让调用方保留原条目。
func archiveMemoryBatch(batchID string, batch memoryBatch) error {
	return archiveMemoryItems(batchID, batch.Items, "consolidation")
}

// archiveMemoryItems 把指定条目追加到归档日志；reason 区分正常整理与待处理兜底。
func archiveMemoryItems(batchID string, items []memoryItem, reason string) error {
	if len(items) == 0 {
		return nil
	}
	path := filepath.Join(config.LocalDir, filepath.FromSlash(memoryArchiveFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建归档目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开归档日志失败: %w", err)
	}
	defer file.Close()

	now := time.Now().Unix()
	for _, item := range items {
		record := memoryArchiveRecord{
			BatchID:    batchID,
			ID:         item.ID,
			Content:    item.Content,
			Role:       item.Role,
			Timestamp:  item.Timestamp,
			Reason:     reason,
			ArchivedAt: now,
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("序列化归档记录失败: %w", err)
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			return fmt.Errorf("写入归档日志失败: %w", err)
		}
	}
	return nil
}
