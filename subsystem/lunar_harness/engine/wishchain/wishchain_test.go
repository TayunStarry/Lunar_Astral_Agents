package wishchain

// wishchain_test.go 心愿链路核心验收：容量与优先级、到期提醒、心情联动、
// 已完成愿望清除、统一持久化文件与情绪锚点。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// fakeEmbed 把情绪标签 / 态度锚点映射到确定性单位向量。
func fakeEmbed(_ context.Context, texts []string) ([][]float32, error) {
	names := append(append([]string{}, labels...), attitudeAnchorNames()...)
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		text = strings.TrimSpace(text)
		pos, ok := index[text]
		if !ok {
			switch {
			case strings.Contains(text, "喜欢"), strings.Contains(text, "信任"), strings.Contains(text, "关心"):
				pos = len(labels)
			case strings.Contains(text, "讨厌"), strings.Contains(text, "反感"), strings.Contains(text, "冷淡"):
				pos = len(labels) + len(positiveLabels)
			default:
				pos = 1 // 非标签文本默认落到「愤怒」方向
			}
		}
		vec := make([]float32, len(names))
		vec[pos] = 1
		out[i] = vec
	}
	return out, nil
}

func newChain(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	Reset()
	oldHook := embedDocumentsHook
	embedDocumentsHook = fakeEmbed
	t.Cleanup(func() {
		Reset()
		embedDocumentsHook = oldHook
		config.LocalDir = oldDir
	})
}

func TestWishPrioritySortAndCapacity(t *testing.T) {
	newChain(t)
	priorities := []int{7, 2, 15, 0, 9, 1, 12, 4, 14, 3, 10, 5, 13, 6, 11, 8}
	for i, priority := range priorities {
		if _, err := Add("愿望"+string(rune('A'+i)), "", priority); err != nil {
			t.Fatalf("新增第 %d 条失败: %v", i+1, err)
		}
	}
	list := List()
	if len(list) != MaxPending {
		t.Fatalf("待实现应为 %d 条，实际 %d", MaxPending, len(list))
	}
	for i, item := range list {
		if item.Priority != i {
			t.Fatalf("第 %d 条优先级应为 %d，实际 %d", i, i, item.Priority)
		}
	}
	if _, err := Add("第 17 条", "", AutoPriority); err != ErrCapacity {
		t.Fatalf("满员后应返回 ErrCapacity，实际 %v", err)
	}
	// 完成一条后腾出位置，自动分配应复用最低可用优先级。
	if _, err := Complete(list[0].ID); err != nil {
		t.Fatal(err)
	}
	added, err := Add("新愿望", "", AutoPriority)
	if err != nil {
		t.Fatalf("腾出位置后应可新增: %v", err)
	}
	if added.Priority != 0 {
		t.Fatalf("自动分配应复用最高优先级 0，实际 %d", added.Priority)
	}
}

func TestDueNotificationAndGrace(t *testing.T) {
	newChain(t)
	now := time.Now()
	item, err := Add("看科幻电影", now.Add(-time.Minute).Format("2006-01-02 15:04:05"), AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	notices := ConsumeDue(now)
	if len(notices) != 1 {
		t.Fatalf("应产生 1 条到期提醒，实际 %d", len(notices))
	}
	if notices[0].Wish.Content != "看科幻电影" {
		t.Fatalf("到期提醒应携带愿望内容，实际 %q", notices[0].Wish.Content)
	}
	if notices[0].Overdue {
		t.Fatal("30 分钟宽限期内不应判定为逾期")
	}
	if again := ConsumeDue(now); len(again) != 0 {
		t.Fatalf("到期提醒不应重复消费: %+v", again)
	}
	// 超出宽限期后结算为逾期。
	settled := Settle(now.Add(GraceDuration + time.Minute))
	found := false
	for _, change := range settled.Applied {
		if change.WishID == item.ID && change.Reason == "overdue" && change.Label == "失望" {
			found = true
		}
	}
	if !found {
		t.Fatalf("超出宽限期应结算为失望，实际 %+v", settled.Applied)
	}
	repeated := Settle(now.Add(GraceDuration + 2*time.Minute))
	repeatedOverdue := false
	for _, change := range repeated.Applied {
		if change.WishID == item.ID && change.Reason == "overdue" {
			repeatedOverdue = true
		}
	}
	if !repeatedOverdue {
		t.Fatalf("逾期应每轮持续结算，实际 %+v", repeated.Applied)
	}
}

func TestSettlementClearsCompletedWishes(t *testing.T) {
	newChain(t)
	item, err := Add("写一首歌", "", AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(item.ID); err != nil {
		t.Fatal(err)
	}
	result := Settle(time.Now())
	if len(result.Applied) != 1 || result.Applied[0].Reason != "completed" || result.Applied[0].Label != "满足" {
		t.Fatalf("完成愿望应结算为满足，实际 %+v", result.Applied)
	}
	if len(result.Removed) != 1 || result.Removed[0] != item.ID {
		t.Fatalf("完成且心情已结算的愿望应被清除，实际 %+v", result.Removed)
	}
	if len(List()) != 0 {
		t.Fatalf("清除后待实现列表应为空，实际 %+v", List())
	}
	if again := Settle(time.Now()); len(again.Applied) != 0 || len(again.Removed) != 0 {
		t.Fatalf("已清除的愿望不应重复结算: %+v", again)
	}
}

func TestPostponedWishSettlesRegret(t *testing.T) {
	newChain(t)
	future := time.Now().Add(24 * time.Hour)
	item, err := Add("整理房间", future.Format("2006-01-02 15:04:05"), AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	later := future.Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := Update(item.ID, "", later, AutoPriority); err != nil {
		t.Fatal(err)
	}
	result := Settle(time.Now())
	found := false
	for _, change := range result.Applied {
		if change.Reason == "postponed" && change.Label == "遗憾" {
			found = true
		}
	}
	if !found {
		t.Fatalf("延期愿望应结算为遗憾，实际 %+v", result.Applied)
	}
}

func TestNearDueSettlesHappyOnce(t *testing.T) {
	newChain(t)
	now := time.Now()
	item, err := Add("看电影", now.Add(30*time.Minute).Format("2006-01-02 15:04:05"), AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	result := Settle(now)
	found := false
	for _, change := range result.Applied {
		if change.WishID == item.ID && change.Reason == "near" && change.Label == "开心" {
			found = true
		}
	}
	if !found {
		t.Fatalf("临近截止应结算为开心，实际 %+v", result.Applied)
	}
	if again := Settle(now.Add(time.Minute)); len(again.Applied) != 0 {
		t.Fatalf("临近联动不应重复结算: %+v", again.Applied)
	}
}

func TestEmotionAnchorsAndNudge(t *testing.T) {
	newChain(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := SnapshotMood().Label; got != "开心" {
		t.Fatalf("初始心情应为开心，实际 %s", got)
	}
	if !NudgeMood("愤怒", 1) {
		t.Fatal("锚点就绪后应能施加心情偏移")
	}
	snapshot := SnapshotMood()
	if snapshot.Label != "愤怒" {
		t.Fatalf("偏移后心情应为愤怒，实际 %s（%v）", snapshot.Label, snapshot.Weights)
	}
	if sum := weightsSum(snapshot.Weights); sum < 1-1e-9 || sum > 1+1e-9 {
		t.Fatalf("心情权重总和应恒为 1，实际 %.12f", sum)
	}
	expected := map[string]bool{
		"开心": true, "愤怒": true, "伤心": true, "好奇": true, "无聊": true,
		"失望": true, "厌倦": true, "担忧": true, "恐慌": true, "满足": true, "遗憾": true,
	}
	if len(snapshot.Weights) != len(expected) {
		t.Fatalf("情绪锚点数量应为 %d，实际 %d", len(expected), len(snapshot.Weights))
	}
	for label := range expected {
		if _, ok := snapshot.Weights[label]; !ok {
			t.Fatalf("缺少情绪锚点 %s", label)
		}
	}
}

func TestCompleteMatchingByTitle(t *testing.T) {
	newChain(t)
	viewWish, err := Add("看《流浪地球》", "", AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	writeWish, err := Add("写一篇关于月亮的散文", "", AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	completed, ok := CompleteMatching("看完《流浪地球》\n科幻\n科幻电影")
	if !ok || completed.ID != viewWish.ID {
		t.Fatalf("应按标题完成观影愿望，实际 %+v（%v）", completed, ok)
	}
	if item, _ := Get(writeWish.ID); item.Status != StatusPending {
		t.Fatalf("无关愿望不应被完成: %+v", item)
	}
}

func TestCompleteMatchingByTopic(t *testing.T) {
	newChain(t)
	wish, err := Add("写一篇关于月亮的散文", "", AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	completed, ok := CompleteMatching("月亮主题散文\n写完一篇月亮主题散文\n创作返回 800 字")
	if !ok || completed.ID != wish.ID {
		t.Fatalf("应按创作主题完成愿望，实际 %+v（%v）", completed, ok)
	}
}

func TestOverdueSettledEveryRoundAndRetained(t *testing.T) {
	newChain(t)
	past := time.Now().Add(-GraceDuration - time.Hour).Format("2006-01-02 15:04:05")
	wish, err := Add("写一篇关于月亮的散文", past, AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		result := Settle(time.Now())
		found := false
		for _, change := range result.Applied {
			if change.WishID == wish.ID && change.Reason == "overdue" && change.Label == "失望" {
				found = true
			}
		}
		if !found {
			t.Fatalf("第 %d 轮应执行逾期结算，实际 %+v", round, result.Applied)
		}
		item, ok := Get(wish.ID)
		if !ok || item.Status != StatusPending {
			t.Fatalf("逾期愿望不应被销毁，实际 %+v（%v）", item, ok)
		}
	}
}

func TestOverdueWishRetainedUntilComplete(t *testing.T) {
	newChain(t)
	past := time.Now().Add(-GraceDuration - time.Hour).Format("2006-01-02 15:04:05")
	wish, err := Add("看《流浪地球》", past, AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	Settle(time.Now())
	if item, ok := Get(wish.ID); !ok || item.Status != StatusPending {
		t.Fatalf("逾期后应保留愿望，实际 %+v（%v）", item, ok)
	}
	if _, err := Complete(wish.ID); err != nil {
		t.Fatal(err)
	}
	Settle(time.Now())
	if _, ok := Get(wish.ID); ok {
		t.Fatal("完成并结算后应清除愿望")
	}
}

func TestOverdueWishesExcludeFuture(t *testing.T) {
	newChain(t)
	past := time.Now().Add(-GraceDuration - time.Hour).Format("2006-01-02 15:04:05")
	if _, err := Add("写一篇关于月亮的散文", past, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := Add("看《流浪地球》", past, 0); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := Add("下个月去海边", future, 1); err != nil {
		t.Fatal(err)
	}
	overdue := OverdueWishes(time.Now())
	if len(overdue) != 2 {
		t.Fatalf("应返回 2 条逾期愿望，实际 %d：%+v", len(overdue), overdue)
	}
	for _, item := range overdue {
		if item.Content == "下个月去海边" {
			t.Fatalf("未逾期愿望不应进入逾期集合: %+v", overdue)
		}
	}
}

func TestUnifiedPersistenceFile(t *testing.T) {
	newChain(t)
	if err := Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	item, err := Add("统一持久化", "", AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateAttitudeWindows(context.Background(), []Round{{
		Key: "a1", UserName: "钛宇星光阁", UserText: "我今天很喜欢你", ReplyText: "我也很喜欢你",
	}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.LocalDir, filepath.FromSlash(stateFile))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("统一持久化文件应存在: %v", err)
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("统一持久化文件应为合法 JSON: %v", err)
	}
	if len(saved.Wishes) != 1 || saved.Wishes[0].ID != item.ID {
		t.Fatalf("愿望未写入统一文件: %+v", saved.Wishes)
	}
	if saved.Mood.Label == "" || len(saved.Attitude.Users) == 0 {
		t.Fatalf("心情与态度应写入同一文件: mood=%q users=%d", saved.Mood.Label, len(saved.Attitude.Users))
	}
}

func TestAttitudePointsAccumulateAndClamp(t *testing.T) {
	newChain(t)
	ctx := context.Background()
	evaluate := func(key, userText, replyText string) {
		t.Helper()
		if _, err := UpdateAttitudeWindows(ctx, []Round{{
			Key: key, UserName: "钛宇星光阁", UserText: userText, ReplyText: replyText,
		}}); err != nil {
			t.Fatal(err)
		}
	}

	// 首次正向交互：默认 20 + 10 = 30，进入「喜欢」。
	evaluate("p1", "我今天很喜欢你", "我也很喜欢你")
	if points, ok := AttitudeScore("钛宇星光阁"); !ok || points != 30 {
		t.Fatalf("首次正向交互后应为 30 分，实际 %d（%v）", points, ok)
	}
	if label, _ := AttitudeLabel("钛宇星光阁"); label != "喜欢" {
		t.Fatalf("30 分应为「喜欢」，实际 %s", label)
	}

	// 同一轮重复提交不重复加分。
	evaluate("p1", "我今天很喜欢你", "我也很喜欢你")
	if points, _ := AttitudeScore("钛宇星光阁"); points != 30 {
		t.Fatalf("重复评估同一轮不应加分，实际 %d", points)
	}

	// 持续正向封顶 50。
	for i := 0; i < 5; i++ {
		evaluate(fmt.Sprintf("p%d", i+2), "我今天很喜欢你", "我也很喜欢你")
	}
	if points, _ := AttitudeScore("钛宇星光阁"); points != attitudeMaxPoints {
		t.Fatalf("正向累积应封顶 %d，实际 %d", attitudeMaxPoints, points)
	}

	// 持续负向触底，标签转为「讨厌」。
	for i := 0; i < 12; i++ {
		evaluate(fmt.Sprintf("n%d", i), "我讨厌你", "我不想理你")
	}
	if points, _ := AttitudeScore("钛宇星光阁"); points != attitudeMinPoints {
		t.Fatalf("负向累积应触底 %d，实际 %d", attitudeMinPoints, points)
	}
	if label, _ := AttitudeLabel("钛宇星光阁"); label != "讨厌" {
		t.Fatalf("%d 分应为「讨厌」，实际 %s", attitudeMinPoints, label)
	}
}

func TestLegacyAttitudeScoreMigrates(t *testing.T) {
	newChain(t)
	legacy := `{"version":1,"attitude":{"users":{"旧友":{"score":-0.8,"label":"平淡","samples":2}}}}`
	if err := persistence.WriteFileAtomic(stateFile, []byte(legacy)); err != nil {
		t.Fatal(err)
	}
	Load()
	points, ok := AttitudeScore("旧友")
	if !ok {
		t.Fatal("旧版好感度应迁移为态度积分")
	}
	if points != -4 {
		t.Fatalf("-0.8 应换算为 -4 分，实际 %d", points)
	}
	if label, _ := AttitudeLabel("旧友"); label != "疏离" {
		t.Fatalf("-4 分应为「疏离」，实际 %s", label)
	}
}
