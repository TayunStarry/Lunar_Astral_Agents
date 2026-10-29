package affection

import (
	"context"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// fakeEmbed 按关键词把文本映射到「正向 / 负向 / 中性」三个方向之一。
func fakeEmbed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		pos := 2 // 默认中性
		switch {
		case strings.Contains(text, "喜欢"), strings.Contains(text, "信任"), strings.Contains(text, "关心"):
			pos = 0
		case strings.Contains(text, "讨厌"), strings.Contains(text, "反感"), strings.Contains(text, "冷淡"):
			pos = 1
		}
		vec := make([]float32, 3)
		vec[pos] = 1
		out[i] = vec
	}
	return out, nil
}

func resetAffection(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	oldHook := embedDocumentsHook
	oldAlpha := currentAlpha
	embedDocumentsHook = fakeEmbed
	currentAlpha = affectionAlpha
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

func TestAffectionPerUserIndependent(t *testing.T) {
	resetAffection(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Update(context.Background(), "钛宇星光阁", "你今天真好看", "我也很喜欢和你聊天呀"); err != nil {
		t.Fatal(err)
	}
	if err := Update(context.Background(), "琉璃", "你走吧", "我讨厌这样，想离远一点"); err != nil {
		t.Fatal(err)
	}
	if label, ok := Label("钛宇星光阁"); !ok || label != "很喜欢" {
		t.Fatalf("钛宇星光阁应为很喜欢，实际 %s（%v）", label, ok)
	}
	if label, ok := Label("琉璃"); !ok || label != "讨厌" {
		t.Fatalf("琉璃应为讨厌，实际 %s（%v）", label, ok)
	}
	if _, ok := Label("陌生人"); ok {
		t.Fatal("未认知的用户不应有情感标签")
	}
}

func TestAffectionEmptyPairIsNoop(t *testing.T) {
	resetAffection(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Update(context.Background(), "钛宇星光阁", "", "我也很喜欢和你聊天呀"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Label("钛宇星光阁"); ok {
		t.Fatal("缺少用户发言时不应形成情感认知")
	}
}

func TestAffectionBlendsTowardsNeutral(t *testing.T) {
	resetAffection(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Update(context.Background(), "钛宇星光阁", "你今天真好看", "我也很喜欢和你聊天呀"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := Update(context.Background(), "钛宇星光阁", "今天天气一般", "普普通通的一天"); err != nil {
			t.Fatal(err)
		}
	}
	if label, _ := Label("钛宇星光阁"); label != defaultLabel {
		t.Fatalf("长期中性交互后应回到平淡，实际 %s", label)
	}
}

func TestAffectionInitRestoresPersistedState(t *testing.T) {
	resetAffection(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 模拟进程重启：只清内存，保留本地持久化文件。
	mu.Lock()
	state = State{}
	initialized = false
	mu.Unlock()
	embedDocumentsHook = func(context.Context, []string) ([][]float32, error) {
		t.Fatal("已有持久化情感认知时不应重新计算锚点")
		return nil, nil
	}
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !initialized {
		t.Fatal("重启后应恢复持久化的情感认知状态")
	}
}
