package viewer

// watch_topic_test.go 观影主题词偏好池：完成才加权、重复标记不重复计、按半衰期过期。
//
// 池的衰减、加权与淘汰策略在 watch_topic.go，因此断言全部留在本包；
// 存储侧（engine/viewerstore）只保留行级读写，不再断言偏好语义。

import (
	"math"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// TestWatchedTopicsFeedPoolOnlyAfterCompletion 只看完的视频才入池，重复标记同一片段不重复加权。
func TestWatchedTopicsFeedPoolOnlyAfterCompletion(t *testing.T) {
	isolateViewerDB(t)
	now := time.Now().Unix()
	candidate := topicCandidate("BV1topic", 2)
	candidate.Topics = []string{"人工智能", "编程", "人工智能"}
	if _, err := viewerstore.EnqueueVideos([]viewerstore.WatchCandidate{candidate}); err != nil {
		t.Fatal(err)
	}
	video, ok, err := viewerstore.LoadVideo("BV1topic")
	if err != nil || !ok {
		t.Fatalf("读取视频失败 ok=%v err=%v", ok, err)
	}
	if len(video.Topics) != 2 {
		t.Fatalf("主题词应去重后保留 2 个，实际 %v", video.Topics)
	}
	if err := viewerstore.SetVideoReview("BV1topic", viewerstore.WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}

	// 只看了第一段：还没「看完」，不应进入偏好池。
	if _, err := viewerstore.MarkSegmentWatched("BV1topic", 0, now, bumpWatchedTopics); err != nil {
		t.Fatal(err)
	}
	if topics, err := activeWatchedTopics(6, now); err != nil || len(topics) != 0 {
		t.Fatalf("未看完不应入池，实际 %v err=%v", topics, err)
	}

	// 看完最后一段：主题词入池。
	if _, err := viewerstore.MarkSegmentWatched("BV1topic", 1, now, bumpWatchedTopics); err != nil {
		t.Fatal(err)
	}
	topics, err := activeWatchedTopics(6, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 || topics[0] != "人工智能" || topics[1] != "编程" {
		t.Fatalf("看完后主题词应入池，实际 %v", topics)
	}

	// 重复标记同一片段：完成状态不再翻转，权重不应叠加。
	if _, err := viewerstore.MarkSegmentWatched("BV1topic", 1, now, bumpWatchedTopics); err != nil {
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
	if topics, err := activeWatchedTopics(6, now); err != nil || len(topics) != 1 {
		t.Fatalf("刚写入应有效，实际 %v err=%v", topics, err)
	}

	halfLife := int64(watchTopicHalfLifeDays * 24 * 60 * 60)
	if topics, err := activeWatchedTopics(6, now+halfLife); err != nil || len(topics) != 1 {
		t.Fatalf("一个半衰期后权重 0.5 仍应有效，实际 %v err=%v", topics, err)
	}
	if topics, err := activeWatchedTopics(6, now+2*halfLife); err != nil || len(topics) != 0 {
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
	topics, err := activeWatchedTopics(6, now)
	if err != nil || len(topics) != 2 || topics[0] != "科幻" {
		t.Fatalf("高权重主题应排在前面，实际 %v err=%v", topics, err)
	}
}

// recordWatchedTopics 直接驱动「读全池 → 算 → 写回」的单事务入口（测试与一次性补偿共用）。
func recordWatchedTopics(topics []string, at int64) error {
	return viewerstore.UpdateTopicPool(func(pool viewerstore.TopicPool) error {
		return bumpWatchedTopics(pool, topics, at)
	})
}

// topicWeightOf 读取主题词池里某个词的存储权重（测试辅助）。
func topicWeightOf(t *testing.T, topic string) float64 {
	t.Helper()
	rows, err := viewerstore.LoadTopicRows(watchTopicPoolMax)
	if err != nil {
		t.Fatalf("读取主题词池失败: %v", err)
	}
	for _, row := range rows {
		if row.Topic == topic {
			return row.Weight
		}
	}
	t.Fatalf("读取主题词权重失败: 未找到 %q", topic)
	return 0
}

// topicCandidate 构造一条带片段的入队候选。
func topicCandidate(bv string, count int) viewerstore.WatchCandidate {
	out := viewerstore.WatchCandidate{
		BV:          bv,
		Title:       "标题 " + bv,
		Intro:       "简介",
		AuthorUID:   "1001",
		AuthorName:  "作者",
		Comments:    []string{"评论一", "评论二"},
		DurationSec: 600,
	}
	for index := 0; index < count; index++ {
		out.Segments = append(out.Segments, viewerstore.SegmentInput{
			StartSec:   index * 30,
			EndSec:     index*30 + 20,
			Path:       "segments/" + string(rune('a'+index)) + ".mp4",
			Downloaded: true,
		})
	}
	return out
}
