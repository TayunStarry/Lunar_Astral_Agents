package creation

// plan_polarity_test.go 心情极性判定：正向 / 负向 / 中性边界。

import (
	"testing"

	"LunarSubsystem/LunarHarness/chain/wishchain"
)

// TestPolarityBoundaries 极性差值达到 1/N（均匀份额）才算明确倾向。
func TestPolarityBoundaries(t *testing.T) {
	const threshold = 1.0 / 11.0
	cases := []struct {
		name    string
		weights map[string]float64
		want    MoodPolarity
	}{
		{
			name:    "正向明确",
			weights: testWeights(map[string]float64{"开心": 0.60, "担忧": 0.10}),
			want:    MoodPositive,
		},
		{
			name:    "负向明确",
			weights: testWeights(map[string]float64{"担忧": 0.60, "开心": 0.10}),
			want:    MoodNegative,
		},
		{
			name:    "正向恰好达到阈值",
			weights: testWeights(map[string]float64{"开心": 0.30, "担忧": 0.30 - threshold}),
			want:    MoodPositive,
		},
		{
			name:    "正向差一点未达阈值",
			weights: testWeights(map[string]float64{"开心": 0.30, "担忧": 0.30 - threshold + 0.001}),
			want:    MoodNeutral,
		},
		{
			name:    "负向恰好达到阈值",
			weights: testWeights(map[string]float64{"担忧": 0.30, "开心": 0.30 - threshold}),
			want:    MoodNegative,
		},
		{
			name:    "负向差一点未达阈值",
			weights: testWeights(map[string]float64{"担忧": 0.30, "开心": 0.30 - threshold + 0.001}),
			want:    MoodNeutral,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := polarityOf(wishchain.MoodSnapshot{Label: "开心", Weights: testCase.weights})
			if got != testCase.want {
				t.Fatalf("极性判定错误：期望 %d，实际 %d", testCase.want, got)
			}
		})
	}
}

// TestPolarityFallsBackToLabel 权重缺失时按主导标签兜底。
func TestPolarityFallsBackToLabel(t *testing.T) {
	if got := polarityOf(wishchain.MoodSnapshot{Label: "伤心"}); got != MoodNegative {
		t.Fatalf("主导标签为负向情绪时应判为偏负，实际 %d", got)
	}
	if got := polarityOf(wishchain.MoodSnapshot{Label: "开心"}); got != MoodPositive {
		t.Fatalf("主导标签为正向情绪时应判为偏正，实际 %d", got)
	}
}

// testWeights 构造完整的 11 类权重表：默认 0.01，按 overrides 覆盖。
func testWeights(overrides map[string]float64) map[string]float64 {
	labels := []string{"开心", "愤怒", "伤心", "好奇", "无聊", "失望", "厌倦", "担忧", "恐慌", "满足", "遗憾"}
	weights := make(map[string]float64, len(labels))
	for _, label := range labels {
		weights[label] = 0.01
	}
	for label, weight := range overrides {
		weights[label] = weight
	}
	return weights
}
