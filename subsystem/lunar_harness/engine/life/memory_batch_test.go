package life

// memory_batch_test.go 记忆整理「条目化」规则的回归测试
//
// 钉住三条可预测性保证：
//  1. 整理后的保留下限随输入条数缩放，并夹在合法区间
//  2. 模型过度压缩时的保底只去重、截短，不丢条目
//  3. 单条整理后记忆有统一字符上限

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestMemoryMinKeep 保留下限：比例缩放 + 上下限夹取
func TestMemoryMinKeep(t *testing.T) {
	cases := []struct {
		count int
		want  int
	}{
		{0, 0},
		{-3, 0},
		{3, 3},                    // 小于下限时最多保留输入本身
		{8, memoryKeepFloor},      // 8 × 25% = 2 → 抬到下限
		{20, 5},                   // 常规比例
		{125, 31},                 // 大批次按比例
		{1000, memoryKeepCeiling}, // 超大批次夹到上限
	}
	for _, c := range cases {
		if got := memoryMinKeep(c.count); got != c.want {
			t.Fatalf("输入 %d 条应保留下限 %d，实得 %d", c.count, c.want, got)
		}
	}
}

// TestFallbackMemoryTexts 保底整理：空条目跳过、完全重复去掉、长条目截短
func TestFallbackMemoryTexts(t *testing.T) {
	long := strings.Repeat("字", memoryMaxTextRunes+30)
	items := []memoryItem{
		{Content: "  今天  和  小想 聊了  插件  "},
		{Content: "今天 和 小想 聊了 插件"}, // 空白折叠后与前一条完全相同
		{Content: "   "},
		{Content: long},
	}
	got := fallbackMemoryTexts(items)
	if len(got) != 2 {
		t.Fatalf("应去重并跳过空条目后剩 2 条，实得 %d 条: %v", len(got), got)
	}
	if got[0] != "今天 和 小想 聊了 插件" {
		t.Fatalf("空白应折叠为单空格，实得 %q", got[0])
	}
	if runes := len([]rune(got[1])); runes > memoryMaxTextRunes+1 {
		t.Fatalf("超长条目应截短到 %d 字以内（含省略号），实得 %d 字", memoryMaxTextRunes+1, runes)
	}
}

// TestParseMemoryArrayTruncates 单条输出超限时截短，且重复内容只保留一次
func TestParseMemoryArrayTruncates(t *testing.T) {
	long := strings.Repeat("字", memoryMaxTextRunes+20)
	raw := `["` + long + `", "` + long + `", "偏好喝无糖乌龙茶"]`
	got, err := parseMemoryArray(raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("重复条应去重，实得 %d 条", len(got))
	}
	if runes := len([]rune(got[0])); runes > memoryMaxTextRunes+1 {
		t.Fatalf("条目应截短到 %d 字以内，实得 %d 字", memoryMaxTextRunes+1, runes)
	}
}

// TestMemoryTaskPromptMentionsLimits 提示词必须把条数与下限明确告诉模型
func TestMemoryTaskPromptMentionsLimits(t *testing.T) {
	prompt := memoryTaskPrompt(125, 31)
	for _, want := range []string{"125", "31", "200", "JSON 字符串数组", "事实", "人物"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("提示词缺少 %q: %s", want, prompt)
		}
	}
}

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
