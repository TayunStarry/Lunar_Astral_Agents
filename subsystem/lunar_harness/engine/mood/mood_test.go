package mood

import (
	"context"
	"fmt"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func fakeEmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	index := make(map[string]int, len(labels))
	for i, label := range labels {
		index[label] = i
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		pos, ok := index[text]
		if !ok {
			pos = 1 // 非标签文本默认当成「愤怒」方向
		}
		vec := make([]float32, len(labels))
		vec[pos] = 1
		out[i] = vec
	}
	return out, nil
}

func resetMood(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	oldHook := embedDocumentsHook
	oldAlpha := currentAlpha
	oldTemperature := currentTemperature
	embedDocumentsHook = fakeEmbedDocuments
	mu.Lock()
	state = State{}
	initialized = false
	mu.Unlock()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		embedDocumentsHook = oldHook
		currentAlpha = oldAlpha
		currentTemperature = oldTemperature
		mu.Lock()
		state = State{}
		initialized = false
		mu.Unlock()
	})
}

func TestInitBiasedHappy(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot()
	if snapshot.Label != "开心" {
		t.Fatalf("初始标签应为开心，实际 %s", snapshot.Label)
	}
	if snapshot.Weights["开心"] < 0.69 {
		t.Fatalf("初始权重应偏向开心，实际 %v", snapshot.Weights)
	}
}

func TestInitRestoresPersistedStateWithoutEmbedding(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 模拟进程重启：只清内存，保留 LocalDir 下的持久化文件。
	mu.Lock()
	state = State{}
	initialized = false
	mu.Unlock()
	embedDocumentsHook = func(context.Context, []string) ([][]float32, error) {
		t.Fatal("已有持久化心情时不应重新计算基础情绪向量")
		return nil, nil
	}
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if label := Snapshot().Label; label != "开心" {
		t.Fatalf("重启后应恢复持久化标签，实际 %s", label)
	}
}

func TestUpdateBlendsTowardsAngry(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	currentAlpha = 0
	if err := UpdateFromText(context.Background(), "今天真的有点生气"); err != nil {
		t.Fatal(err)
	}
	if label := Snapshot().Label; label != "愤怒" {
		t.Fatalf("更新后标签应为愤怒，实际 %s", label)
	}
}

// TestInitRepairsLegacyLabel 旧状态文件里标签可能与权重不一致；
// 重新加载时应按权重校准并持久化，用户无需等到下一次发言才看到变化。
func TestInitRepairsLegacyLabel(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	state.Label = "开心"
	state.Weights = map[string]float64{"元气": 0.8, "开心": 0.2}
	saveStateLocked()
	state = State{}
	initialized = false
	mu.Unlock()

	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := Snapshot().Label; got != "元气" {
		t.Fatalf("加载旧状态时应按权重校准标签为元气，实际 %s", got)
	}
}

// TestLabelFollowsTopWeightWithInertia 钉住标签与权重的一致性：
// 即使心情向量仍带有旧心情的惯性，标签也必须取当前权重最高者。
func TestLabelFollowsTopWeightWithInertia(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 默认 alpha=0.7：初始向量偏向开心，直接更新到元气时旧实现会仍返回开心。
	if err := UpdateFromText(context.Background(), "元气"); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot()
	if snapshot.Weights["元气"] <= snapshot.Weights["开心"] {
		t.Fatalf("前置条件不成立：元气权重应高于开心，实际 %v", snapshot.Weights)
	}
	if snapshot.Label != "元气" {
		t.Fatalf("标签应跟随最高权重「元气」，实际 %s（权重 %v）", snapshot.Label, snapshot.Weights)
	}
}

// TestUpdateSupportsExpandedLabels 新增的细分情绪都能作为独立标签参与匹配。
func TestUpdateSupportsExpandedLabels(t *testing.T) {
	for _, label := range []string{"平静", "厌倦", "担忧", "恐慌", "满足", "元气"} {
		t.Run(label, func(t *testing.T) {
			resetMood(t)
			if err := Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			currentAlpha = 0
			if err := UpdateFromText(context.Background(), label); err != nil {
				t.Fatal(err)
			}
			if got := Snapshot().Label; got != label {
				t.Fatalf("标签应为 %s，实际 %s", label, got)
			}
		})
	}
}

// TestMoodAmplitudeBounded 校验放大后的默认幅度：
// 单次更新至少把情绪向量与目标情绪的余弦推进 0.25，且连续切换后仍是单位向量（不越界、不发散）。
func TestMoodAmplitudeBounded(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if currentAlpha > 0.35 || currentTemperature > 0.10 {
		t.Fatalf("默认幅度未放大：alpha=%v temperature=%v", currentAlpha, currentTemperature)
	}
	before := Snapshot()
	if err := UpdateFromText(context.Background(), "愤怒"); err != nil {
		t.Fatal(err)
	}
	after := Snapshot()
	mu.RLock()
	target := append([]float32(nil), state.Base["愤怒"]...)
	base := copyBase(state.Base)
	mu.RUnlock()

	progress := cosine(after.Vector, target) - cosine(before.Vector, target)
	if progress < 0.25 || progress > 1.0 {
		t.Fatalf("单次情绪推进幅度应落在 [0.25, 1.0]，实际 %.3f", progress)
	}
	assertUnitVector(t, after.Vector)
	if sum := weightsSum(after.Weights); sum < 1-1e-9 || sum > 1+1e-9 {
		t.Fatalf("情绪权重总和应恒为 1，实际 %.12f", sum)
	}

	// 连续在两种极端情绪间切换，验证值域始终有界。
	for i := 0; i < 20; i++ {
		text := "愤怒"
		if i%2 == 0 {
			text = "元气"
		}
		if err := UpdateFromText(context.Background(), text); err != nil {
			t.Fatal(err)
		}
		snapshot := Snapshot()
		assertUnitVector(t, snapshot.Vector)
		for label, vec := range base {
			if sim := cosine(snapshot.Vector, vec); sim < -1-1e-6 || sim > 1+1e-6 {
				t.Fatalf("余弦越界：%s=%.6f", label, sim)
			}
		}
	}
}

func assertUnitVector(t *testing.T, vec []float32) {
	t.Helper()
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum < 1-1e-3 || sum > 1+1e-3 {
		t.Fatalf("情绪向量应为单位向量，模长平方=%.6f", sum)
	}
}

func weightsSum(weights map[string]float64) float64 {
	var sum float64
	for _, v := range weights {
		sum += v
	}
	return sum
}

func TestUpdateKeepsMoodOnEmbeddingFailure(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := Snapshot().Label
	embedDocumentsHook = func(context.Context, []string) ([][]float32, error) {
		return nil, fmt.Errorf("embedding down")
	}
	if err := UpdateFromText(context.Background(), "随便说点什么"); err == nil {
		t.Fatal("嵌入失败时应返回错误")
	}
	if after := Snapshot().Label; after != before {
		t.Fatalf("嵌入失败应保留旧心情，%s → %s", before, after)
	}
}

func TestUpdateEmptyTextIsNoop(t *testing.T) {
	resetMood(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := Snapshot().Label
	if err := UpdateFromText(context.Background(), "   "); err != nil {
		t.Fatal(err)
	}
	if after := Snapshot().Label; after != before {
		t.Fatalf("空文本不应改变心情，%s → %s", before, after)
	}
}
