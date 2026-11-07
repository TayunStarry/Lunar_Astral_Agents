package viewerstore

// watchtopic.go 观看主题词偏好池（watch_topic）的读写。
//
// 反馈环：视频看完时把印象主题词按 +1 权重并入池 → 选片时读衰减后的高频词作为动态偏好。
// 池只在写入时增量聚合；读取只对 ≤128 行做时间衰减算术，不回扫 watch_video 历史。

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ActiveTopics 返回当前有效主题词，按衰减后的权重降序取前 limit 个。
//
// at 为用于衰减计算的 Unix 秒；<=0 时取当前时间。
func ActiveTopics(limit int, at int64) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT topic, weight, updated_at FROM watch_topic`)
	if err != nil {
		return nil, fmt.Errorf("读取主题词池失败: %w", err)
	}
	defer rows.Close()

	list := make([]topicWeight, 0, watchTopicPoolMax)
	for rows.Next() {
		var (
			topic     string
			weight    float64
			updatedAt int64
		)
		if err := rows.Scan(&topic, &weight, &updatedAt); err != nil {
			return nil, fmt.Errorf("解析主题词池失败: %w", err)
		}
		effective := decayTopicWeight(weight, updatedAt, at)
		if effective < watchTopicActiveMinWeight {
			continue
		}
		list = append(list, topicWeight{Topic: topic, Weight: effective})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取主题词池失败: %w", err)
	}
	sortTopicWeights(list)
	if len(list) > limit {
		list = list[:limit]
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, item.Topic)
	}
	return out, nil
}

// recordWatchedTopics 把一个已看完视频的主题词并入偏好池（补偿与测试复用同一份聚合实现）。
func recordWatchedTopics(topics []string, at int64) error {
	normalized := normalizeTopics(topics)
	if len(normalized) == 0 {
		return nil
	}
	if at <= 0 {
		at = time.Now().Unix()
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
	if err := bumpTopicWeights(tx, normalized, at); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交主题词事务失败: %w", err)
	}
	return nil
}

// bumpTopicWeights 在事务内把主题词并入池：先全池衰减到 at，再逐个加权，最后淘汰与截断。
func bumpTopicWeights(tx *sql.Tx, topics []string, at int64) error {
	normalized := normalizeTopics(topics)
	if len(normalized) == 0 {
		return nil
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	entries := make(map[string]*topicWeight, watchTopicPoolMax)
	rows, err := tx.Query(`SELECT topic, weight, updated_at FROM watch_topic`)
	if err != nil {
		return fmt.Errorf("读取主题词池失败: %w", err)
	}
	for rows.Next() {
		var (
			topic     string
			weight    float64
			updatedAt int64
		)
		if err := rows.Scan(&topic, &weight, &updatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("解析主题词池失败: %w", err)
		}
		entries[strings.ToLower(topic)] = &topicWeight{
			Topic:  topic,
			Weight: decayTopicWeight(weight, updatedAt, at),
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("读取主题词池失败: %w", err)
	}
	rows.Close()

	for _, topic := range normalized {
		key := strings.ToLower(topic)
		if entry, ok := entries[key]; ok {
			entry.Weight += watchTopicBump
			continue
		}
		entries[key] = &topicWeight{Topic: topic, Weight: watchTopicBump}
	}

	list := make([]topicWeight, 0, len(entries))
	for _, entry := range entries {
		if entry.Weight < watchTopicMinWeight {
			continue
		}
		list = append(list, *entry)
	}
	sortTopicWeights(list)
	if len(list) > watchTopicPoolMax {
		list = list[:watchTopicPoolMax]
	}

	if _, err := tx.Exec(`DELETE FROM watch_topic`); err != nil {
		return fmt.Errorf("清理主题词池失败: %w", err)
	}
	for _, entry := range list {
		if _, err := tx.Exec(`INSERT INTO watch_topic (topic, weight, updated_at)
			VALUES (?, ?, ?)`, entry.Topic, entry.Weight, at); err != nil {
			return fmt.Errorf("写入主题词池失败: %w", err)
		}
	}
	return nil
}

// decayTopicWeight 按半衰期把权重衰减到 at 时刻。
func decayTopicWeight(weight float64, updatedAt, at int64) float64 {
	if weight <= 0 {
		return 0
	}
	if updatedAt <= 0 || at <= updatedAt {
		return weight
	}
	halfLife := float64(watchTopicHalfLifeDays * 24 * 60 * 60)
	return weight * math.Pow(0.5, float64(at-updatedAt)/halfLife)
}

// sortTopicWeights 按权重降序排列，权重相同按主题词字典序保证结果稳定。
func sortTopicWeights(list []topicWeight) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Weight == list[j].Weight {
			return list[i].Topic < list[j].Topic
		}
		return list[i].Weight > list[j].Weight
	})
}
