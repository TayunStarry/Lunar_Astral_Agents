package creation

// plan_next_test.go 自主选题：心情续做 / 换新、随机覆盖与排除窗口、QQ 可用性与活动状态。

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/mood"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// TestPlanNextResumesUnfinishedOnPositive 偏正向时重复调用返回同一未完成事项。
func TestPlanNextResumesUnfinishedOnPositive(t *testing.T) {
	setupPlanTest(t, positiveWeights(), qqTargets{}, 7)

	first := mustPlan(t)
	second := mustPlan(t)
	if first.ID != second.ID {
		t.Fatalf("心情偏正向时应续做未完成事项，首次 %s，第二次 %s", first.ID, second.ID)
	}
	if second.Reason == "" {
		t.Fatal("事项应带有选定依据")
	}
}

// TestPlanNextNegativeSwitchesAndPreservesPending 偏负向时换新，旧事项保留并可恢复。
func TestPlanNextNegativeSwitchesAndPreservesPending(t *testing.T) {
	setupPlanTest(t, positiveWeights(), qqTargets{}, 11)

	pending := mustPlan(t)
	setMoodWeights(negativeWeights())
	fresh := mustPlan(t)
	if fresh.ID == pending.ID {
		t.Fatal("心情偏负向时应换成一个新事项")
	}

	state := loadPlanState()
	if state.Active == nil || state.Active.ID != fresh.ID {
		t.Fatal("新事项应成为当前未完成事项")
	}
	if len(state.Parked) != 1 || state.Parked[0].ID != pending.ID {
		t.Fatalf("旧事项应被保留在暂存队列，实际 %+v", state.Parked)
	}

	// 心情转正：先续做当前事项
	setMoodWeights(positiveWeights())
	if resumed := mustPlan(t); resumed.ID != fresh.ID {
		t.Fatalf("心情偏正向时应先续做当前事项 %s，实际 %s", fresh.ID, resumed.ID)
	}
	// 完成当前事项后，暂存的旧事项应被恢复
	finishPlan(fresh)
	if restored := mustPlan(t); restored.ID != pending.ID {
		t.Fatalf("暂存事项应被恢复为 %s，实际 %s", pending.ID, restored.ID)
	}
	if state = loadPlanState(); len(state.Parked) != 0 {
		t.Fatalf("恢复后暂存队列应清空，实际 %+v", state.Parked)
	}
}

// TestQQUnavailableSkipsJokeKind QQ 适配器不可用时不得选到笑话事项，也不报错。
func TestQQUnavailableSkipsJokeKind(t *testing.T) {
	setupPlanTest(t, positiveWeights(), qqTargets{}, 23)
	for i := 0; i < 60; i++ {
		item := mustPlan(t)
		if item.Kind == KindJoke {
			t.Fatalf("QQ 不可用时不应选中笑话事项：%+v", item)
		}
		finishPlan(item)
	}
}

// TestCatalogCoversAllKindsWithRecentWindow 随机抽取覆盖全部类型，且遵守最近排除窗口。
func TestCatalogCoversAllKindsWithRecentWindow(t *testing.T) {
	targets := qqTargets{Items: []qqTarget{{Group: true, ID: 10001, Name: "测试群"}, {ID: 20002, Name: "小明"}}}
	setupPlanTest(t, positiveWeights(), targets, 31)

	seen := make(map[ItemKind]bool, len(itemSpecs))
	kinds := make([]ItemKind, 0, 120)
	for i := 0; i < 120; i++ {
		item := mustPlan(t)
		if strings.TrimSpace(item.Task) == "" || len(item.Params) == 0 {
			t.Fatalf("事项参数不完整：%+v", item)
		}
		seen[item.Kind] = true
		kinds = append(kinds, item.Kind)
		finishPlan(item)
	}

	if len(seen) != len(itemSpecs) {
		t.Fatalf("随机抽取应覆盖全部 %d 种事项，实际覆盖 %d 种", len(itemSpecs), len(seen))
	}
	for i := 2; i < len(kinds); i++ {
		if kinds[i] == kinds[i-1] || kinds[i] == kinds[i-2] {
			t.Fatalf("第 %d 次抽到 %s，违反最近 %d 次排除窗口：%v", i, kinds[i], recentWindow, kinds)
		}
	}
}

// TestEveryItemKindBuildsExecutableParams 每种事项都能生成完整可执行的参数。
func TestEveryItemKindBuildsExecutableParams(t *testing.T) {
	targets := qqTargets{Items: []qqTarget{{Group: true, ID: 10001, Name: "测试群"}}}
	rng := rand.New(rand.NewSource(5))
	requiredKeys := map[ItemKind][]string{
		KindNovel:    {"题材", "篇幅", "视角"},
		KindResearch: {"事件", "角度"},
		KindProject:  {"方向", "技术形态"},
		KindStory:    {"基调", "篇幅", "主角"},
		KindTidy:     {"目录", "整理动作"},
		KindJoke:     {"对象类型", "对象", "对象ID", "风格"},
		KindPaint:    {"题材", "风格", "画幅", "批次"},
	}

	for _, spec := range itemSpecs {
		item, ok := spec.Build(rng, targets)
		if !ok {
			t.Fatalf("QQ 可用时 %s 事项应当可以生成", spec.Label)
		}
		if item.Kind != spec.Kind || strings.TrimSpace(item.Title) == "" ||
			strings.TrimSpace(item.Task) == "" || strings.TrimSpace(item.Activity) == "" {
			t.Fatalf("%s 事项信息不完整：%+v", spec.Label, item)
		}
		for _, key := range requiredKeys[spec.Kind] {
			if strings.TrimSpace(item.Params[key]) == "" {
				t.Fatalf("%s 事项缺少参数 %q：%+v", spec.Label, key, item.Params)
			}
		}
	}

	// QQ 不可用时，笑话事项必须拒绝生成而不是产出不可执行的任务
	for _, spec := range itemSpecs {
		if spec.Kind != KindJoke {
			continue
		}
		if _, ok := spec.Build(rng, qqTargets{}); ok {
			t.Fatal("QQ 适配器不可用时不应生成笑话事项")
		}
	}
}

// TestJokeDeliveryTargets 笑话事项按群 / 好友选择正确的投递工具与目标编号。
func TestJokeDeliveryTargets(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	groupItem, ok := buildJoke(rng, qqTargets{Items: []qqTarget{{Group: true, ID: 10001, Name: "测试群"}}})
	if !ok || !strings.Contains(groupItem.Task, toolSendGroupText) || !strings.Contains(groupItem.Task, "10001") {
		t.Fatalf("群聊笑话应使用 %s 并带群号：%+v", toolSendGroupText, groupItem)
	}
	friendItem, ok := buildJoke(rng, qqTargets{Items: []qqTarget{{ID: 20002, Name: "小明"}}})
	if !ok || !strings.Contains(friendItem.Task, toolSendPrivateText) || !strings.Contains(friendItem.Task, "20002") {
		t.Fatalf("好友笑话应使用 %s 并带 QQ 号：%+v", toolSendPrivateText, friendItem)
	}
}

// TestFinishPlanUpdatesActivity 事项完成后立即刷新「我刚刚在干的事情」。
func TestFinishPlanUpdatesActivity(t *testing.T) {
	setupPlanTest(t, positiveWeights(), qqTargets{}, 3)
	item := PlanItem{ID: "activity-case", Kind: KindStory, Activity: "刚写完一篇小故事"}
	finishPlan(item)
	if got := activity.Current(); got != item.Activity {
		t.Fatalf("活动状态应更新为 %q，实际 %q", item.Activity, got)
	}
}

// setupPlanTest 隔离数据目录并替换心情 / QQ / 随机源钩子。
func setupPlanTest(t *testing.T, weights map[string]float64, targets qqTargets, seed int64) {
	t.Helper()
	testenv.Isolate(t)
	rng := rand.New(rand.NewSource(seed))
	oldMood, oldQQ, oldRand := moodSnapshotHook, qqProbeHook, newRandHook
	setMoodWeights(weights)
	qqProbeHook = func() qqTargets { return targets }
	newRandHook = func() *rand.Rand { return rng }
	t.Cleanup(func() {
		moodSnapshotHook, qqProbeHook, newRandHook = oldMood, oldQQ, oldRand
	})
}

// setMoodWeights 替换心情快照，模拟心情在正负之间切换。
func setMoodWeights(weights map[string]float64) {
	moodSnapshotHook = func() mood.MoodSnapshot {
		return mood.MoodSnapshot{Label: "开心", Weights: weights}
	}
}

// positiveWeights 正向明确的心情权重（极性差值远高于 1/11）。
func positiveWeights() map[string]float64 {
	return testWeights(map[string]float64{"开心": 0.60, "担忧": 0.10})
}

// negativeWeights 负向明确的心情权重。
func negativeWeights() map[string]float64 {
	return testWeights(map[string]float64{"担忧": 0.60, "开心": 0.10})
}

// mustPlan 调用入口并要求成功。
func mustPlan(t *testing.T) PlanItem {
	t.Helper()
	item, err := PlanNext(context.Background())
	if err != nil {
		t.Fatalf("自主选题失败: %v", err)
	}
	return item
}
