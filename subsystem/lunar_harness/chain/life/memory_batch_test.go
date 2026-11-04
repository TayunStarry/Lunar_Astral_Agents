package life

// memory_batch_test.go 记忆整理「候选挑选」规则的回归测试
//
// 条目化整理（保留下限 / 保底 / 解析 / 提示词）已随模型调用迁入 agents/LifeMemorizer，
// 对应用例在 agents/LifeMemorizer/memory_test.go；本文件只钉住链侧的触发条件与取批规则。

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestSelectMemoryCandidates 触发条件：≥500 条、取最久 50 条、全部满 10 天
func TestSelectMemoryCandidates(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)

	// 不足 500 条：不触发
	if got, reason := selectMemoryCandidates(makeMemoryItems(499, 30, now), now); len(got) != 0 {
		t.Fatalf("499 条不应触发，实得 %d 条", len(got))
	} else if !strings.Contains(reason, "500") {
		t.Fatalf("原因应提到触发线 500，实得 %q", reason)
	}

	// 满 500 条但全部不满 10 天：不触发
	if got, reason := selectMemoryCandidates(makeMemoryItems(500, 1, now), now); len(got) != 0 {
		t.Fatalf("全部新记忆不应触发，实得 %d 条", len(got))
	} else if !strings.Contains(reason, "10") {
		t.Fatalf("原因应提到 10 天门槛，实得 %q", reason)
	}

	// 满 500 条但满 10 天的只有 49 条：不触发
	items := makeMemoryItems(49, 12, now)
	items = append(items, makeMemoryItems(451, 1, now)...)
	if got, reason := selectMemoryCandidates(items, now); len(got) != 0 {
		t.Fatalf("满 10 天不足 50 条不应触发，实得 %d 条", len(got))
	} else if !strings.Contains(reason, "50") {
		t.Fatalf("原因应提到单次 50 条，实得 %q", reason)
	}

	// 满 10 天的有 60 条：只取最久的 50 条
	items = makeMemoryItems(60, 12, now)
	items = append(items, makeMemoryItems(440, 1, now)...)
	got, reason := selectMemoryCandidates(items, now)
	if len(got) != memoryBatchItems {
		t.Fatalf("应选 %d 条，实得 %d 条（reason=%q）", memoryBatchItems, len(got), reason)
	}
	for i, item := range got {
		if item.ID != fmt.Sprintf("item-%03d", i) {
			t.Fatalf("第 %d 条应是最旧的 item-%03d，实得 %s", i, i, item.ID)
		}
	}

	// 缺失时间戳按最久远处理
	legacy := makeMemoryItems(500, 0, now)
	for i := range legacy {
		legacy[i].Timestamp = 0
	}
	if got, _ := selectMemoryCandidates(legacy, now); len(got) != memoryBatchItems {
		t.Fatalf("缺失时间戳的旧数据应可整理，实得 %d 条", len(got))
	}
}

// makeMemoryItems 生成按入库时间升序的测试条目；ageDays 为入库距今天数（0 表示现在）
func makeMemoryItems(count int, ageDays int, now time.Time) []memoryItem {
	items := make([]memoryItem, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, memoryItem{
			ID:        fmt.Sprintf("item-%03d", i),
			Content:   fmt.Sprintf("记忆 %03d", i),
			Timestamp: now.Add(-time.Duration(ageDays) * 24 * time.Hour).Add(time.Duration(i) * time.Second).Unix(),
		})
	}
	return items
}
