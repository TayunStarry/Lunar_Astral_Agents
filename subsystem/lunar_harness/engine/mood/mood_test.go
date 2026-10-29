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
	embedDocumentsHook = fakeEmbedDocuments
	mu.Lock()
	state = State{}
	initialized = false
	mu.Unlock()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		embedDocumentsHook = oldHook
		currentAlpha = oldAlpha
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
