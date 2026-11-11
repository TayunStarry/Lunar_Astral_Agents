package viewer

// watch_topic.go 观看主题词偏好池的业务策略：衰减、加权、淘汰、截断与读取筛选。
//
// 存储侧（engine/viewerstore）只提供 watch_topic 的行级读写与单事务回调入口；
// 这里决定「月华怎么记观影偏好」，数值常量见 variable.go。

import (
	"math"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// topicWeight 主题词偏好池里一条参与打分的记录（权重已衰减到目标时刻）。
type topicWeight struct {
	Topic  string
	Weight float64
}

// activeWatchedTopics 返回当前有效主题词，按衰减后的权重降序取前 limit 个。
//
// at 为用于衰减计算的 Unix 秒；<=0 时取当前时间。
func activeWatchedTopics(limit int, at int64) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	rows, err := viewerstore.LoadTopicRows(watchTopicPoolMax)
	if err != nil {
		return nil, err
	}
	list := make([]topicWeight, 0, len(rows))
	for _, row := range rows {
		effective := decayTopicWeight(row.Weight, row.UpdatedAt, at)
		if effective < watchTopicActiveMinWeight {
			continue
		}
		list = append(list, topicWeight{Topic: row.Topic, Weight: effective})
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

// bumpWatchedTopics 在偏好池事务内把主题词并入池：先全池衰减到 at，再逐个加权，最后淘汰与截断。
//
// 作为 viewerstore.MarkSegmentWatched 的完成回调使用，复用存储侧的标记事务；
// 单独的「读改写」走 viewerstore.UpdateTopicPool。
func bumpWatchedTopics(pool viewerstore.TopicPool, topics []string, at int64) error {
	normalized := viewerstore.NormalizeTopics(topics)
	if len(normalized) == 0 {
		return nil
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	rows, err := pool.Load(watchTopicPoolMax)
	if err != nil {
		return err
	}
	entries := make(map[string]*topicWeight, watchTopicPoolMax)
	for _, row := range rows {
		entries[strings.ToLower(row.Topic)] = &topicWeight{
			Topic:  row.Topic,
			Weight: decayTopicWeight(row.Weight, row.UpdatedAt, at),
		}
	}

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

	next := make([]viewerstore.TopicRow, 0, len(list))
	for _, entry := range list {
		next = append(next, viewerstore.TopicRow{Topic: entry.Topic, Weight: entry.Weight, UpdatedAt: at})
	}
	return pool.Replace(next)
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
