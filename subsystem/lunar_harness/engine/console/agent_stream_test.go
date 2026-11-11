package console

import (
	"strings"
	"testing"
)

// 覆盖：工具调用必须归到**当前这一轮**，并且该轮随后能正常收尾。
//
// 回归背景（用户实测）：实况屏上出现「只有一条 dispatch_painter、状态永远进行中」的轮次，
// 对话者明明已经答完（日志里 TTS 已推送文本），界面却一直停在"进行中"。
// 根因是流式桥先 StreamEnd 再上报工具：StreamEnd 已把当前轮归档并清空 currentTurn，
// 随后的 StreamTool 便**另起一轮**且从不结束 —— 幽灵轮次。
func TestStreamToolBelongsToCurrentTurn(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindContent, "画好了")
	StreamTool("对话者", "portrait", `{"character":"琉璃","chibi":true}`)
	StreamEnd("对话者")

	turns := GetTurns()
	if len(turns) != 1 {
		t.Fatalf("应只有 1 轮，实得 %d 轮（多出来的就是幽灵轮次）", len(turns))
	}
	if turns[0].Active {
		t.Fatal("已结束的轮次不应标记为进行中")
	}
	if len(turns[0].Tools) != 1 || !strings.HasPrefix(turns[0].Tools[0], "portrait") {
		t.Fatalf("工具调用应记录在当轮里，实得 %#v", turns[0].Tools)
	}
	if turns[0].Content != "画好了" {
		t.Fatalf("正文应留在当轮，实得 %q", turns[0].Content)
	}
	if info := GetTurnStoreInfo(); info.HasActive {
		t.Fatal("结束后不应存在进行中的轮次（否则实况屏永远显示进行中）")
	}
}

// 覆盖：不同智能体各写各的轮次，实况里不会互相串味。
//
// 回归背景（用户实测）：记忆合并整理的文本被拼进「对话者」的条目里，且不是另起一条，
// 而是接在对话者已经结束的那一轮后面继续写。根因是 attachTurnLocked 忽略角色、
// 一律挂到当前/最近一轮；现在只挂本角色自己的轮次。
func TestStreamKeepsRolesInSeparateTurns(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindContent, "我先说")
	StreamEnd("对话者")

	StreamBegin("记忆整理")
	StreamDelta("记忆整理", KindContent, "八条重复记忆合并为一条")
	StreamEnd("记忆整理")

	turns := GetTurns()
	if len(turns) != 2 {
		t.Fatalf("应各自成轮，共 2 轮，实得 %d 轮", len(turns))
	}
	if turns[0].Role != "对话者" || turns[0].Content != "我先说" {
		t.Fatalf("对话者轮次被污染: role=%q content=%q", turns[0].Role, turns[0].Content)
	}
	if turns[1].Role != "记忆整理" || turns[1].Content != "八条重复记忆合并为一条" {
		t.Fatalf("记忆整理应独立成轮: role=%q content=%q", turns[1].Role, turns[1].Content)
	}
}

// 覆盖：没有 Begin 的增量也不能落到别的智能体轮次里（防止再次出现串味）。
func TestStreamDeltaWithoutOwnTurnDoesNotLeak(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindContent, "我先说")
	StreamEnd("对话者")

	// 模拟旧实现里静默调用只发增量、不发 Begin 的情况
	StreamDelta("记忆整理", KindContent, "这段不该并进对话者")

	turns := GetTurns()
	if len(turns) != 1 {
		t.Fatalf("迟到增量不应造轮，实得 %d 轮", len(turns))
	}
	if strings.Contains(turns[0].Content, "不该并进") {
		t.Fatalf("异角色增量混进了对话者轮次: %q", turns[0].Content)
	}
}

// 覆盖：迟到的工具/增量事件不得凭空造轮。
//
// 即便流式桥将来又把上报顺序写反，最坏也只应把内容并进最近一轮，
// 而不是留下一个没有结束事件的"进行中"轮次（那会让界面永久卡在进行中）。
func TestLateToolEventDoesNotFabricateTurn(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindContent, "第一句")
	StreamEnd("对话者")

	// 归档之后迟到的工具事件（历史缺陷里正是这个顺序）
	StreamTool("对话者", "list_images", `{"limit":5}`)
	StreamDelta("对话者", KindContent, "迟到的一句")

	turns := GetTurns()
	if len(turns) != 1 {
		t.Fatalf("迟到事件不应新增轮次，实得 %d 轮", len(turns))
	}
	if turns[0].Active {
		t.Fatal("迟到事件不应把轮次重新标成进行中")
	}
	if len(turns[0].Tools) != 1 {
		t.Fatalf("迟到工具应并入最近一轮，实得 %#v", turns[0].Tools)
	}
	if !strings.HasSuffix(turns[0].Content, "迟到的一句") {
		t.Fatalf("迟到增量应并入最近一轮，实得 %q", turns[0].Content)
	}
}
