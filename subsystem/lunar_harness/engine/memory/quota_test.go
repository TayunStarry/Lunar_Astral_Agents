package memory

import "testing"

// quotaFixture 构造 count 条属于 speaker 的命中，分数从 base 起递减。
func quotaFixture(speaker string, count int, base float32, startID int) []QuotaHit {
	hits := make([]QuotaHit, 0, count)
	for i := 0; i < count; i++ {
		hits = append(hits, QuotaHit{
			ID:         speaker + "-" + string(rune('a'+startID+i)),
			SpeakerKey: speaker,
			Timestamp:  int64(1000 + i),
			Score:      base - float32(i)*0.01,
		})
	}
	return hits
}

func TestAllocateQuotaFiveUsers(t *testing.T) {
	speakers := []string{"u1", "u2", "u3", "u4", "u5"}
	var hits []QuotaHit
	for i, speaker := range speakers {
		hits = append(hits, quotaFixture(speaker, 7, float32(0.9-float64(i)*0.05), 0)...)
	}
	result := AllocateQuota(hits, speakers, QuotaOptions{})
	for _, speaker := range speakers {
		if got := len(result.PerUser[speaker]); got != QuotaPerUser {
			t.Fatalf("%s 配额应为 %d，实际 %d", speaker, QuotaPerUser, got)
		}
	}
	if got := len(result.Global); got != QuotaTopK-QuotaPerUser*len(speakers) {
		t.Fatalf("全局配额应为 %d，实际 %d", QuotaTopK-QuotaPerUser*len(speakers), got)
	}
	total := len(result.Global)
	for _, items := range result.PerUser {
		total += len(items)
	}
	if total != QuotaTopK {
		t.Fatalf("总数应为 %d，实际 %d", QuotaTopK, total)
	}
}

func TestAllocateQuotaFewerParticipantsMergesIntoGlobal(t *testing.T) {
	speakers := []string{"u1", "u2", "u3"}
	var hits []QuotaHit
	for _, speaker := range speakers {
		hits = append(hits, quotaFixture(speaker, 12, 0.9, 0)...)
	}
	result := AllocateQuota(hits, speakers, QuotaOptions{})
	if got := len(result.Global); got != QuotaTopK-QuotaPerUser*len(speakers) {
		t.Fatalf("人数不足时余额应并入全局: 期望 %d 实际 %d", QuotaTopK-QuotaPerUser*len(speakers), got)
	}
}

func TestAllocateQuotaCapsParticipants(t *testing.T) {
	speakers := []string{"u1", "u2", "u3", "u4", "u5", "u6"}
	var hits []QuotaHit
	for _, speaker := range speakers {
		hits = append(hits, quotaFixture(speaker, 6, 0.9, 0)...)
	}
	result := AllocateQuota(hits, speakers, QuotaOptions{})
	if _, ok := result.PerUser["u6"]; ok {
		t.Fatal("超过上限的参与者不应获得独立配额")
	}
	if len(result.PerUser) != QuotaMaxParticipants {
		t.Fatalf("应只有 %d 个分组，实际 %d", QuotaMaxParticipants, len(result.PerUser))
	}
}

func TestAllocateQuotaRespectsTopK(t *testing.T) {
	var hits []QuotaHit
	for _, speaker := range []string{"u1", "u2", "u3"} {
		hits = append(hits, quotaFixture(speaker, 6, 0.9, 0)...)
	}
	result := AllocateQuota(hits, []string{"u1", "u2", "u3"}, QuotaOptions{TopK: 4})
	total := len(result.Global)
	for _, items := range result.PerUser {
		total += len(items)
	}
	if total != 4 {
		t.Fatalf("总量应被 TopK 截断为 4，实际 %d", total)
	}
}

func TestAllocateQuotaOrdersByScore(t *testing.T) {
	hits := []QuotaHit{
		{ID: "low", SpeakerKey: "u1", Score: 0.2, Timestamp: 1},
		{ID: "high", SpeakerKey: "u1", Score: 0.9, Timestamp: 2},
		{ID: "mid", SpeakerKey: "u1", Score: 0.5, Timestamp: 3},
	}
	result := AllocateQuota(hits, []string{"u1"}, QuotaOptions{})
	got := result.PerUser["u1"]
	if len(got) != 3 || got[0].ID != "high" || got[1].ID != "mid" || got[2].ID != "low" {
		t.Fatalf("组内应按分数降序: %+v", got)
	}
}

func TestDeduplicateHits(t *testing.T) {
	hits := []QuotaHit{
		{ID: "a", Content: "同一句话"},
		{ID: "a", Content: "另一句"},
		{ID: "b", Content: "同一句话"},
		{ID: "c", Content: "同一   句话"},
		{ID: "d", Content: "不同"},
	}
	got := DeduplicateHits(hits)
	if len(got) != 3 || got[0].ID != "a" || got[1].ID != "c" || got[2].ID != "d" {
		t.Fatalf("去重结果不符: %+v", got)
	}
}

func TestContentHashNormalizesWhitespace(t *testing.T) {
	if ContentHash("你好  世界") != ContentHash(" 你好 世界 ") {
		t.Fatal("空白差异应视为同一正文")
	}
	if ContentHash("你好") == ContentHash("你好世界") {
		t.Fatal("不同正文不应同哈希")
	}
	if ContentHash("   ") != "" {
		t.Fatal("空正文哈希应为空串")
	}
}

func TestSortHitsByTime(t *testing.T) {
	hits := []QuotaHit{
		{ID: "unknown", Timestamp: 0},
		{ID: "late", Timestamp: 300},
		{ID: "early", Timestamp: 100},
		{ID: "same-b", Timestamp: 200},
		{ID: "same-a", Timestamp: 200},
	}
	SortHitsByTime(hits)
	want := []string{"early", "same-a", "same-b", "late", "unknown"}
	for i, id := range want {
		if hits[i].ID != id {
			t.Fatalf("时间排序不符: 期望 %v 实际 %+v", want, hits)
		}
	}
}
