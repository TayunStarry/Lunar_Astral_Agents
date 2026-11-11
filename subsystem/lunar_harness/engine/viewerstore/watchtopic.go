package viewerstore

// watchtopic.go 观看主题词偏好池（watch_topic）的行级读写。
//
// 存储层只负责表结构与按行读写：衰减、加权、淘汰、排序与筛选等偏好策略在 chain/viewer。
// 池的每条记录由主题词、权重与最后一次写入时间组成，读取一律带行数上限。

import (
	"database/sql"
	"fmt"
)

// LoadTopicRows 读取主题词池，最多 limit 行（limit <= 0 返回错误，避免一次拉全表）。
func LoadTopicRows(limit int) ([]TopicRow, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	return loadTopicRows(db, limit)
}

// UpdateTopicPool 在单个事务内执行回调，回调里的 Load / Replace 共享同一事务。
//
// 回调返回错误时整个事务回滚；这是偏好池「读全池 → 算 → 写回」的唯一入口，
// 不允许调用方拆成两次独立事务，否则并发写会丢更新。
func UpdateTopicPool(fn func(TopicPool) error) error {
	if fn == nil {
		return nil
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始主题词事务失败: %w", err)
	}
	defer tx.Rollback()
	if err := fn(topicPoolTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交主题词事务失败: %w", err)
	}
	return nil
}

// topicPoolTx 把一个事务绑定成 TopicPool。
type topicPoolTx struct {
	tx *sql.Tx
}

// Load 在事务内读取主题词池。
func (p topicPoolTx) Load(limit int) ([]TopicRow, error) {
	return loadTopicRows(p.tx, limit)
}

// Replace 在事务内用 rows 整体覆盖主题词池。
func (p topicPoolTx) Replace(rows []TopicRow) error {
	if _, err := p.tx.Exec(`DELETE FROM watch_topic`); err != nil {
		return fmt.Errorf("清理主题词池失败: %w", err)
	}
	for _, row := range rows {
		if _, err := p.tx.Exec(`INSERT INTO watch_topic (topic, weight, updated_at)
			VALUES (?, ?, ?)`, row.Topic, row.Weight, row.UpdatedAt); err != nil {
			return fmt.Errorf("写入主题词池失败: %w", err)
		}
	}
	return nil
}

// topicQueryer 事务内与事务外共用的查询入口（*sql.DB 与 *sql.Tx 都满足）。
type topicQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// loadTopicRows 读取最多 limit 行原始记录。
func loadTopicRows(q topicQueryer, limit int) ([]TopicRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("主题词池读取上限必须为正: %d", limit)
	}
	rows, err := q.Query(`SELECT topic, weight, updated_at FROM watch_topic LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("读取主题词池失败: %w", err)
	}
	defer rows.Close()
	out := make([]TopicRow, 0, limit)
	for rows.Next() {
		var row TopicRow
		if err := rows.Scan(&row.Topic, &row.Weight, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("解析主题词池失败: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取主题词池失败: %w", err)
	}
	return out, nil
}
