package thinking

import (
	"strings"
	"sync"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
)

// fastAckPushRecord 一次快速应答推送的快照。
type fastAckPushRecord struct {
	kind    string
	content string
	audio   string
	auto    bool
}

// captureFastAck 替换快速应答推送入口，记录每次推送，测试结束恢复。
func captureFastAck(t *testing.T) func() []fastAckPushRecord {
	t.Helper()
	previous := fastAckPushHook
	var mu sync.Mutex
	records := make([]fastAckPushRecord, 0, 4)
	fastAckPushHook = func(kind, content, audio string, auto bool) bool {
		mu.Lock()
		records = append(records, fastAckPushRecord{kind: kind, content: content, audio: audio, auto: auto})
		mu.Unlock()
		return true
	}
	t.Cleanup(func() { fastAckPushHook = previous })
	return func() []fastAckPushRecord {
		mu.Lock()
		defer mu.Unlock()
		return append([]fastAckPushRecord{}, records...)
	}
}

// TestFastAckTriggersForCompositeTools 三个复合工具各命中一次。
func TestFastAckTriggersForCompositeTools(t *testing.T) {
	for _, name := range []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolNetworkExplore, Dialogue.ToolUseTheProgram} {
		t.Run(name, func(t *testing.T) {
			resetDialogueGlobals(t)
			pushes := captureFastAck(t)
			maybeSendFastAck(name)
			records := pushes()
			if len(records) != 1 {
				t.Fatalf("复合工具 %s 应推送 1 次，实际 %d 次", name, len(records))
			}
			if records[0].kind != "text" || records[0].content == "" {
				t.Fatalf("快速应答应为非空文本，实际 kind=%q content=%q", records[0].kind, records[0].content)
			}
		})
	}
}

// TestFastAckIgnoresOtherTools 非复合工具不触发快速应答。
func TestFastAckIgnoresOtherTools(t *testing.T) {
	for _, name := range []string{"read_file", "notify_user", "unknown_tool", ""} {
		t.Run(name, func(t *testing.T) {
			resetDialogueGlobals(t)
			pushes := captureFastAck(t)
			maybeSendFastAck(name)
			if records := pushes(); len(records) != 0 {
				t.Fatalf("非复合工具 %q 不应推送，实际 %d 次", name, len(records))
			}
		})
	}
}

// TestFastAckOncePerRound 同一轮重复调用（同类或不同类复合工具）只推一次。
func TestFastAckOncePerRound(t *testing.T) {
	cases := []struct {
		name  string
		calls []string
	}{
		{"同工具重复", []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolDispatchTechnique}},
		{"不同复合工具", []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolNetworkExplore}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetDialogueGlobals(t)
			pushes := captureFastAck(t)
			for _, name := range tc.calls {
				maybeSendFastAck(name)
			}
			if records := pushes(); len(records) != 1 {
				t.Fatalf("同一轮只应推送 1 次，实际 %d 次", len(records))
			}
		})
	}
}

// TestFastAckConcurrentCallsPushOnce 并发调用时判定与置位保持原子：同轮仍只推一次。
func TestFastAckConcurrentCallsPushOnce(t *testing.T) {
	resetDialogueGlobals(t)
	pushes := captureFastAck(t)
	useSpeechSynth(t, func(text string) string { return "audio:" + text })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			maybeSendFastAck(Dialogue.ToolDispatchTechnique)
		}()
	}
	wg.Wait()

	if records := pushes(); len(records) != 1 {
		t.Fatalf("并发调用只应推送 1 次，实际 %d 次", len(records))
	}
}

// TestFastAckSpokenAndAutoMarked 快速应答照常朗读，且恒定标记 auto=true（不消费 QQ 应答份额）。
func TestFastAckSpokenAndAutoMarked(t *testing.T) {
	resetDialogueGlobals(t)
	pushes := captureFastAck(t)
	useSpeechSynth(t, func(text string) string { return "audio:" + text })

	maybeSendFastAck(Dialogue.ToolDispatchTechnique)

	records := pushes()
	if len(records) != 1 {
		t.Fatalf("快速应答应推送 1 次，实际 %d 次", len(records))
	}
	if records[0].audio == "" {
		t.Fatal("快速应答应走 TTS，音频不应为空")
	}
	if !records[0].auto {
		t.Fatal("快速应答是旁白，应标记 auto=true")
	}
}

// TestFastAckLinePoolIsToolLevel 分支参数不再参与台词选择：候选池按工具名固定，
// 每次随机取用；且每个复合工具至少两条候选。
func TestFastAckLinePoolIsToolLevel(t *testing.T) {
	for _, name := range []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolNetworkExplore, Dialogue.ToolUseTheProgram} {
		t.Run(name, func(t *testing.T) {
			pool := fastAckLines[name]
			if len(pool) < 2 {
				t.Fatalf("复合工具 %s 至少需要 2 条候选，实际 %d 条", name, len(pool))
			}
			for i := 0; i < 60; i++ {
				text := fastAckText(name)
				if strings.TrimSpace(text) == "" {
					t.Fatalf("第 %d 次应答为空消息", i)
				}
				if strings.TrimSpace(strings.Trim(text, "，。！？～、 ")) == "" {
					t.Fatalf("第 %d 次应答只剩标点：%q", i, text)
				}
				// 三段可能各自缺席，但至少有一段取自本工具的候选池
				if !containsAnyAckLine(text, fastAckOpenings) &&
					!containsAnyAckLine(text, pool) &&
					!containsAnyAckLine(text, fastAckEndings) {
					t.Fatalf("第 %d 次应答未取自 %s 的候选池：%q", i, name, text)
				}
			}
		})
	}
}

// TestJoinAckPartsKeepsFixedOrder 三段固定为「开场 → 动作 → 收尾」，不再互相交换。
func TestJoinAckPartsKeepsFixedOrder(t *testing.T) {
	for i := 0; i < 50; i++ {
		text := joinAckParts([]string{"开场", "动作", "收尾"})
		opening := strings.Index(text, "开场")
		action := strings.Index(text, "动作")
		ending := strings.Index(text, "收尾")
		if opening < 0 || action <= opening || ending <= action {
			t.Fatalf("第 %d 次拼装顺序不是固定顺序：%q", i, text)
		}
	}
}

// TestJoinAckPartsSkipsBlankParts 缺席的段落被直接跳过，只剩一段时也不会拼出标点。
func TestJoinAckPartsSkipsBlankParts(t *testing.T) {
	if got := joinAckParts([]string{"", "", ""}); got != "" {
		t.Fatalf("三段全空时应返回空串交由调用方兜底，实际 %q", got)
	}
	for _, single := range []string{"开场", "动作", "收尾"} {
		got := joinAckParts([]string{"", single, ""})
		if !strings.Contains(got, single) {
			t.Fatalf("只剩 %q 时应保留该段，实际 %q", single, got)
		}
		if strings.TrimSpace(strings.Trim(got, "，。！？～、 ")) == "" {
			t.Fatalf("只剩 %q 时拼出了纯标点：%q", single, got)
		}
	}
}

// TestFastAckNeverEmptyWhenAllPartsDropped 三段同时被舍弃时用正文兜底，绝不出空消息。
func TestFastAckNeverEmptyWhenAllPartsDropped(t *testing.T) {
	restore := forceAckDrops(t, 1, 1, 1)
	defer restore()

	for _, name := range []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolNetworkExplore, Dialogue.ToolUseTheProgram} {
		text := fastAckText(name)
		if strings.TrimSpace(text) == "" {
			t.Fatalf("%s 三段全舍弃时应由正文兜底，实际为空", name)
		}
		if !containsAnyAckLine(text, fastAckLines[name]) {
			t.Fatalf("%s 的兜底文案应取自正文候选，实际 %q", name, text)
		}
	}
}

// TestFastAckPartsDropByChance 舍弃概率确实生效：首尾比正文更容易缺席。
func TestFastAckPartsDropByChance(t *testing.T) {
	const rounds = 400
	dropped := func(items []string, chance float64) int {
		count := 0
		for i := 0; i < rounds; i++ {
			if pickAckPart(items, chance) == "" {
				count++
			}
		}
		return count
	}
	openings := dropped(fastAckOpenings, fastAckOpeningDrop)
	endings := dropped(fastAckEndings, fastAckEndingDrop)
	actions := dropped(fastAckLines[Dialogue.ToolDispatchTechnique], fastAckActionDrop)

	// 期望值分别约为 180 / 220 / 60，取宽松下界避免偶发抖动
	if openings < 120 || endings < 160 {
		t.Fatalf("首尾舍弃概率未生效：开场 %d/%d、收尾 %d/%d", openings, rounds, endings, rounds)
	}
	if actions > openings || actions > endings {
		t.Fatalf("正文是语义主干，舍弃次数应少于首尾：正文 %d、开场 %d、收尾 %d", actions, openings, endings)
	}
}

// containsAnyAckLine 判断文本是否包含候选池中的任意一条台词。
func containsAnyAckLine(text string, items []string) bool {
	for _, item := range items {
		if item != "" && strings.Contains(text, item) {
			return true
		}
	}
	return false
}

// forceAckDrops 临时改写三段舍弃概率，返回恢复函数。
func forceAckDrops(t *testing.T, opening, action, ending float64) func() {
	t.Helper()
	prevOpening, prevAction, prevEnding := fastAckOpeningDrop, fastAckActionDrop, fastAckEndingDrop
	fastAckOpeningDrop, fastAckActionDrop, fastAckEndingDrop = opening, action, ending
	return func() {
		fastAckOpeningDrop, fastAckActionDrop, fastAckEndingDrop = prevOpening, prevAction, prevEnding
	}
}
