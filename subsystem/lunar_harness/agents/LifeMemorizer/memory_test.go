package LifeMemorizer

// memory_test.go 记忆整理「条目化」规则的回归测试（自 chain/life 迁入，用例不变）
//
// 钉住三条可预测性保证：
//  1. 整理后的保留下限随输入条数缩放，并夹在合法区间
//  2. 模型过度压缩时的保底只去重、截短，不丢条目
//  3. 单条整理后记忆有统一字符上限

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// TestMinKeep 保留下限：比例缩放 + 上下限夹取
func TestMinKeep(t *testing.T) {
	cases := []struct {
		count int
		want  int
	}{
		{0, 0},
		{-3, 0},
		{3, 3},              // 小于下限时最多保留输入本身
		{8, KeepFloor},      // 8 × 50% = 4 → 抬到下限 8
		{20, 10},            // 常规比例
		{125, KeepCeiling},  // 125 × 50% = 62 → 夹到上限 48
		{1000, KeepCeiling}, // 超大批次夹到上限
	}
	for _, c := range cases {
		if got := minKeepFor(c.count); got != c.want {
			t.Fatalf("输入 %d 条应保留下限 %d，实得 %d", c.count, c.want, got)
		}
	}
}

// TestFallbackTexts 保底整理：空条目跳过、完全重复去掉、长条目截短
func TestFallbackTexts(t *testing.T) {
	long := strings.Repeat("字", MaxTextRunes+30)
	contents := []string{
		"  今天  和  小想 聊了  插件  ",
		"今天 和 小想 聊了 插件", // 空白折叠后与前一条完全相同
		"   ",
		long,
	}
	got := fallbackTexts(contents)
	if len(got) != 2 {
		t.Fatalf("应去重并跳过空条目后剩 2 条，实得 %d 条: %v", len(got), got)
	}
	if got[0] != "今天 和 小想 聊了 插件" {
		t.Fatalf("空白应折叠为单空格，实得 %q", got[0])
	}
	if runes := len([]rune(got[1])); runes > MaxTextRunes+1 {
		t.Fatalf("超长条目应截短到 %d 字以内（含省略号），实得 %d 字", MaxTextRunes+1, runes)
	}
}

// TestParseArrayTruncates 单条输出超限时截短，且重复内容只保留一次
func TestParseArrayTruncates(t *testing.T) {
	long := strings.Repeat("字", MaxTextRunes+20)
	raw := `["` + long + `", "` + long + `", "偏好喝无糖乌龙茶"]`
	got, err := parseArray(raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("重复条应去重，实得 %d 条", len(got))
	}
	if runes := len([]rune(got[0])); runes > MaxTextRunes+1 {
		t.Fatalf("条目应截短到 %d 字以内，实得 %d 字", MaxTextRunes+1, runes)
	}
}

// TestParseArrayWithFence 代码块包裹的 JSON 数组同样可取用并去重
func TestParseArrayWithFence(t *testing.T) {
	text := "```json\n[\"第一条\", \"第二条\", \"第一条\"]\n```"
	values, err := parseArray(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "第一条" || values[1] != "第二条" {
		t.Fatalf("解析结果异常: %#v", values)
	}
}

// TestRetainEnough 正文总量下限：过度压缩视为不合格，正常合并通过
func TestRetainEnough(t *testing.T) {
	contents := []string{strings.Repeat("甲", 100), strings.Repeat("乙", 100)}
	if retainEnough(contents, []string{strings.Repeat("丙", 10)}) {
		t.Fatal("输出正文仅占输入 5%%，应判定为过度压缩")
	}
	if !retainEnough(contents, []string{strings.Repeat("丙", 45)}) {
		t.Fatal("输出正文占输入 22.5%%，应判定为合格")
	}
}

// TestTruncateAtSentenceKeepsContent 长文本首个逗号出现过早时，不得只保留开头一小段
func TestTruncateAtSentenceKeepsContent(t *testing.T) {
	long := "开头， " + strings.Repeat("内容", 400)
	got := truncateAtSentence(long, MaxTextRunes)
	if runes := len([]rune(got)); runes < MaxTextRunes-2 {
		t.Fatalf("过早句读不应丢掉大半内容，实得 %d 字", runes)
	}
}

// TestConsolidateFallsBackOnGarbage 模型返回非 JSON 时保留全部原条目，不得清空
func TestConsolidateFallsBackOnGarbage(t *testing.T) {
	withFakeModel(t, "抱歉，我无法整理这些记忆")
	contents := []string{"今天和月华聊了插件开发", "小想喜欢无糖乌龙茶", "周末计划去看海"}
	got, err := Consolidate(context.Background(), contents)
	if err != nil {
		t.Fatalf("保底整理不应报错: %v", err)
	}
	assertSameTexts(t, fallbackTexts(contents), got)
}

// TestConsolidateFallsBackOnEmptyArray 模型返回空数组时同样保留原条目
func TestConsolidateFallsBackOnEmptyArray(t *testing.T) {
	withFakeModel(t, "[]")
	contents := []string{"第一条真实记忆", "第二条真实记忆", "第三条真实记忆"}
	got, err := Consolidate(context.Background(), contents)
	if err != nil {
		t.Fatalf("保底整理不应报错: %v", err)
	}
	assertSameTexts(t, fallbackTexts(contents), got)
}

// withFakeModel 注入固定回复的模型端点，测试结束后恢复
func withFakeModel(t *testing.T, content string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
	t.Cleanup(server.Close)
	agentbase.SetModelEndpointProvider(func() agentbase.ModelEndpoint {
		return agentbase.ModelEndpoint{URL: server.URL, Model: "test-model"}
	})
	agentbase.ResetModelCache()
	t.Cleanup(func() {
		agentbase.SetModelEndpointProvider(nil)
		agentbase.ResetModelCache()
	})
}

// assertSameTexts 比较两组整理结果（顺序与内容都要一致）
func assertSameTexts(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("条目数量不一致：want %d，got %d（%v）", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("第 %d 条不一致：want %q，got %q", i, want[i], got[i])
		}
	}
}

// TestTaskPromptMentionsLimits 提示词必须把条数与下限明确告诉模型
func TestTaskPromptMentionsLimits(t *testing.T) {
	prompt := TaskPrompt(125, 31)
	for _, want := range []string{"125", "31", "200", "JSON 字符串数组", "事实", "人物"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("提示词缺少 %q: %s", want, prompt)
		}
	}
}
