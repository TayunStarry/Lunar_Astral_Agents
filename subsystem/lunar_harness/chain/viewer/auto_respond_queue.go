package viewer

// auto_respond_queue.go 自动响应的待回应队列：先落盘再发布，失败可重试。
//
// 队列只由 chain/viewer 自己维护（不改 engine/viewerstore 的结构），
// 默认落在 local_data/database/bilibili/autorespond.json。
//
// 语义：
//   - 新条目先入队并立即落盘，再尝试生成与发布；
//   - 发布成功才移出队列，失败保留并累加尝试次数，下一轮仍能取到；
//   - 连续失败达到上限后移出队列、写入 Dropped（含原因），不再无限重试。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// respondDropTTL 放弃记录保留时长，过期清理，避免文件无限增长。
	respondDropTTL = 30 * 24 * time.Hour
	// respondDropMax 放弃记录条数上限（超出时保留最新）。
	respondDropMax = 200
)

// respondEntry 队列里一条待回应的互动。
type respondEntry struct {
	Source    string    `json:"source"`
	Item      inboxItem `json:"item"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
	AddedAt   int64     `json:"added_at"`
}

// respondDrop 达到重试上限后被放弃的条目（保留原因便于排查）。
type respondDrop struct {
	Source   string `json:"source"`
	User     string `json:"user,omitempty"`
	Message  string `json:"message,omitempty"`
	Attempts int    `json:"attempts"`
	Reason   string `json:"reason,omitempty"`
	Time     int64  `json:"time"`
}

// respondQueue 待回应队列的持久化结构。
type respondQueue struct {
	Pending []respondEntry `json:"pending"`
	Dropped []respondDrop  `json:"dropped,omitempty"`
}

// autoRespondStoreFile 队列文件路径；测试可替换。
var autoRespondStoreFile = func() string {
	return filepath.Join(stateDir(), "autorespond.json")
}

// loadRespondQueue 读取默认位置的队列。
func loadRespondQueue() (respondQueue, error) {
	return loadRespondQueueFile(autoRespondStoreFile())
}

// loadRespondQueueFile 读取指定路径的队列；文件缺失视为空队列，损坏则重建。
func loadRespondQueueFile(path string) (respondQueue, error) {
	queue := respondQueue{Pending: []respondEntry{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return queue, nil
		}
		return queue, err
	}
	if err := json.Unmarshal(raw, &queue); err != nil {
		logger.Warn("BilibiliViewer", "自动响应队列文件损坏，已重建: %v", err)
		return respondQueue{Pending: []respondEntry{}}, nil
	}
	queue.pruneDropped(time.Now())
	return queue, nil
}

// saveRespondQueue 写入默认位置的队列。
func saveRespondQueue(queue respondQueue) error {
	return saveRespondQueueFile(autoRespondStoreFile(), queue)
}

// saveRespondQueueFile 原子落盘到指定路径。
func saveRespondQueueFile(path string, queue respondQueue) error {
	if queue.Pending == nil {
		queue.Pending = []respondEntry{}
	}
	raw, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化自动响应队列失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建自动响应队列目录失败: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("写入自动响应队列失败: %w", err)
	}
	return nil
}

// enqueue 加入一条待回应条目；同一互动已在队列里时不重复入队。
func (q *respondQueue) enqueue(source string, item inboxItem) bool {
	key := respondKey(item)
	for _, entry := range q.Pending {
		if respondKey(entry.Item) == key {
			return false
		}
	}
	q.Pending = append(q.Pending, respondEntry{Source: source, Item: item, AddedAt: time.Now().Unix()})
	return true
}

// removeAt 移除指定下标的条目。
func (q *respondQueue) removeAt(index int) {
	if index < 0 || index >= len(q.Pending) {
		return
	}
	q.Pending = append(q.Pending[:index], q.Pending[index+1:]...)
}

// drop 记录一条达到重试上限、不再重试的条目。
func (q *respondQueue) drop(entry respondEntry, reason string) {
	q.Dropped = append(q.Dropped, respondDrop{
		Source:   entry.Source,
		User:     entry.Item.User,
		Message:  truncate(entry.Item.Message, autoRespondErrorRunes),
		Attempts: entry.Attempts,
		Reason:   truncate(reason, autoRespondErrorRunes),
		Time:     time.Now().Unix(),
	})
	q.pruneDropped(time.Now())
}

// pruneDropped 清理过期与超量的放弃记录。
func (q *respondQueue) pruneDropped(now time.Time) {
	if len(q.Dropped) == 0 {
		return
	}
	cutoff := now.Add(-respondDropTTL).Unix()
	kept := q.Dropped[:0]
	for _, record := range q.Dropped {
		if record.Time >= cutoff {
			kept = append(kept, record)
		}
	}
	q.Dropped = kept
	if len(q.Dropped) <= respondDropMax {
		return
	}
	q.Dropped = append([]respondDrop(nil), q.Dropped[len(q.Dropped)-respondDropMax:]...)
}

// respondKey 一条互动的稳定去重键（与消息中心去重键同源）。
func respondKey(item inboxItem) string {
	if item.MessageID != "" {
		return "msg:" + item.MessageID
	}
	if item.SourceID != "" {
		return fmt.Sprintf("src:%d:%s:%s", item.Type, item.Oid, item.SourceID)
	}
	return fmt.Sprintf("obj:%d:%s:%s", item.Type, item.Oid, strings.TrimSpace(item.Message))
}
