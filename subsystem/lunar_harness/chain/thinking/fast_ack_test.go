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
// 每次随机取用，且每个复合工具至少两条候选。
func TestFastAckLinePoolIsToolLevel(t *testing.T) {
	for _, name := range []string{Dialogue.ToolDispatchTechnique, Dialogue.ToolNetworkExplore, Dialogue.ToolUseTheProgram} {
		t.Run(name, func(t *testing.T) {
			pool := fastAckLines[name]
			if len(pool) < 2 {
				t.Fatalf("复合工具 %s 至少需要 2 条候选，实际 %d 条", name, len(pool))
			}
			for i := 0; i < 30; i++ {
				text := fastAckText(name)
				matched := false
				for _, line := range pool {
					if line != "" && strings.Contains(text, line) {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("第 %d 次应答未取自 %s 的候选池：%q", i, name, text)
				}
			}
		})
	}
}
