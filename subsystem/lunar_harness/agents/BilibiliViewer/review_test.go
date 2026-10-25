package BilibiliViewer

import "testing"

// TestParseVideoReviewJSON 钉住结构化审核输出：通过/原因/切片数都要解析出来。
func TestParseVideoReviewJSON(t *testing.T) {
	pass, err := parseVideoReview(`{"pass": true, "reason": "", "slices": 6}`, 8)
	if err != nil {
		t.Fatalf("解析审核 JSON 失败: %v", err)
	}
	if !pass.Pass || pass.Slices != 6 {
		t.Fatalf("通过结果不符: %#v", pass)
	}

	blocked, err := parseVideoReview("```json\n{\"pass\": false, \"reason\": \"色情低俗\", \"slices\": 5}\n```", 8)
	if err != nil {
		t.Fatalf("解析代码块包裹的审核 JSON 失败: %v", err)
	}
	if blocked.Pass || blocked.Reason != "色情低俗" {
		t.Fatalf("未通过结果不符: %#v", blocked)
	}
}

// TestParseVideoReviewFallback 兼容模型没按 JSON 输出的旧式文本。
func TestParseVideoReviewFallback(t *testing.T) {
	safe, err := parseVideoReview("SAFE 5", 8)
	if err != nil || !safe.Pass || safe.Slices != 5 {
		t.Fatalf("SAFE 兜底解析不符: %#v / %v", safe, err)
	}
	risky, err := parseVideoReview("RISK 色情低俗", 8)
	if err != nil || risky.Pass || risky.Reason == "" {
		t.Fatalf("RISK 兜底解析不符: %#v / %v", risky, err)
	}
	if _, err := parseVideoReview("完全看不懂的输出", 8); err == nil {
		t.Fatal("无法解析的输出应返回错误（调用方按高风险处理）")
	}
}

// TestClampReviewSlices 钉住切片数收敛：2-8，且不超过视频实际可用片段数。
func TestClampReviewSlices(t *testing.T) {
	cases := []struct{ duration, want, expect int }{
		{0, 0, defaultSlicesPerVideo},
		{3600, 99, sliceMaxCount},
		{3600, 1, sliceMinCount},
		{90, 8, 2},  // 90 秒只有 2 个可用片段
		{30, 8, 1},  // 半分钟只有 1 个
		{300, 5, 5}, // 5 分钟正好 5 个
	}
	for _, c := range cases {
		if got := clampReviewSlices(c.duration, c.want); got != c.expect {
			t.Fatalf("duration=%d want=%d 应为 %d，实际 %d", c.duration, c.want, c.expect, got)
		}
	}
}
