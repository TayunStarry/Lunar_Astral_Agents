package viewerstore

// watchtopic_test.go 观影主题词回流：完成才加权、重复标记不重复计、按半衰期过期。

import (
	"math"
	"testing"
	"time"
)

// TestWatchedTopicsFeedPoolOnlyAfterCompletion 只看完的视频才入池，重复标记同一片段不重复加权。
func TestWatchedTopicsFeedPoolOnlyAfterCompletion(t *testing.T) {
	isolateViewerDB(t)
	now := time.Now().Unix()
	candidate := watchCandidate("BV1topic", segments(2)...)
	candidate.Topics = []string{"人工智能", "编程", "人工智能"}
	if _, err := EnqueueVideos([]WatchCandidate{candidate}); err != nil {
		t.Fatal(err)
	}
	video, ok, err := LoadVideo("BV1topic")
	if err != nil || !ok {
		t.Fatalf("读取视频失败 ok=%v err=%v", ok, err)
	}
	if len(video.Topics) != 2 {
		t.Fatalf("主题词应去重后保留 2 个，实际 %v", video.Topics)
	}
	if err := SetVideoReview("BV1topic", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}

	// 只看了第一段：还没「看完」，不应进入偏好池。
	if _, err := MarkSegmentWatched("BV1topic", 0, now); err != nil {
		t.Fatal(err)
	}
	if topics, err := ActiveTopics(6, now); err != nil || len(topics) != 0 {
		t.Fatalf("未看完不应入池，实际 %v err=%v", topics, err)
	}

	// 看完最后一段：主题词入池。
	if _, err := MarkSegmentWatched("BV1topic", 1, now); err != nil {
		t.Fatal(err)
	}
	topics, err := ActiveTopics(6, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 || topics[0] != "人工智能" || topics[1] != "编程" {
		t.Fatalf("看完后主题词应入池，实际 %v", topics)
	}

	// 重复标记同一片段：完成状态不再翻转，权重不应叠加。
	if _, err := MarkSegmentWatched("BV1topic", 1, now); err != nil {
		t.Fatal(err)
	}
	if weight := topicWeightOf(t, "人工智能"); math.Abs(weight-1) > 1e-9 {
		t.Fatalf("重复标记不应重复加权，实际权重 %f", weight)
	}
}

// TestTopicWeightsDecayAndRevive 权重按半衰期衰减到阈值以下即过期，再次写入时从衰减值回升。
func TestTopicWeightsDecayAndRevive(t *testing.T) {
	isolateViewerDB(t)
	now := time.Now().Unix()
	if err := recordWatchedTopics([]string{"科幻"}, now); err != nil {
		t.Fatal(err)
	}
	if topics, err := ActiveTopics(6, now); err != nil || len(topics) != 1 {
		t.Fatalf("刚写入应有效，实际 %v err=%v", topics, err)
	}

	halfLife := int64(watchTopicHalfLifeDays * 24 * 60 * 60)
	if topics, err := ActiveTopics(6, now+halfLife); err != nil || len(topics) != 1 {
		t.Fatalf("一个半衰期后权重 0.5 仍应有效，实际 %v err=%v", topics, err)
	}
	if topics, err := ActiveTopics(6, now+2*halfLife); err != nil || len(topics) != 0 {
		t.Fatalf("两个半衰期后权重 0.25 应过期，实际 %v err=%v", topics, err)
	}

	// 过期后再次写入：先衰减到 0.25 再加 1，得到 1.25。
	if err := recordWatchedTopics([]string{"科幻"}, now+2*halfLife); err != nil {
		t.Fatal(err)
	}
	if weight := topicWeightOf(t, "科幻"); math.Abs(weight-1.25) > 1e-9 {
		t.Fatalf("衰减后再写入应从衰减值继续累加，实际权重 %f", weight)
	}
}

// TestTopicPoolAccumulates 多个视频命中同一主题时权重累加，并共享同一份去重规则。
func TestTopicPoolAccumulates(t *testing.T) {
	isolateViewerDB(t)
	now := time.Now().Unix()
	if err := recordWatchedTopics([]string{"科幻", "科幻"}, now); err != nil {
		t.Fatal(err)
	}
	if err := recordWatchedTopics([]string{"科幻", "纪录片"}, now); err != nil {
		t.Fatal(err)
	}
	if weight := topicWeightOf(t, "科幻"); math.Abs(weight-2) > 1e-9 {
		t.Fatalf("同视频内重复词只计一次、跨视频累加，实际权重 %f", weight)
	}
	topics, err := ActiveTopics(6, now)
	if err != nil || len(topics) != 2 || topics[0] != "科幻" {
		t.Fatalf("高权重主题应排在前面，实际 %v err=%v", topics, err)
	}
}

// topicWeightOf 读取主题词池里某个词的权重（测试辅助）。
func topicWeightOf(t *testing.T, topic string) float64 {
	t.Helper()
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	var weight float64
	if err := db.QueryRow(`SELECT weight FROM watch_topic WHERE topic = ?`, topic).Scan(&weight); err != nil {
		t.Fatalf("读取主题词权重失败: %v", err)
	}
	return weight
}
