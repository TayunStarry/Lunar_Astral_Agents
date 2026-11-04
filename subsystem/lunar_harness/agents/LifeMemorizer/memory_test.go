package LifeMemorizer

// memory_test.go 记忆整理「条目化」规则的回归测试（自 chain/life 迁入，用例不变）
//
// 钉住三条可预测性保证：
//  1. 整理后的保留下限随输入条数缩放，并夹在合法区间
//  2. 模型过度压缩时的保底只去重、截短，不丢条目
//  3. 单条整理后记忆有统一字符上限

import (
	"strings"
	"testing"
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
		{8, KeepFloor},      // 8 × 25% = 2 → 抬到下限
		{20, 5},             // 常规比例
		{125, 31},           // 大批次按比例
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

// TestTaskPromptMentionsLimits 提示词必须把条数与下限明确告诉模型
func TestTaskPromptMentionsLimits(t *testing.T) {
	prompt := TaskPrompt(125, 31)
	for _, want := range []string{"125", "31", "200", "JSON 字符串数组", "事实", "人物"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("提示词缺少 %q: %s", want, prompt)
		}
	}
}
