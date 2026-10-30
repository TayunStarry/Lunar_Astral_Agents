package console

import (
	"testing"
)

// 覆盖：数据导出屏的每个可导出对象都必须是**列表里可选的一行**，
// 并且底栏键位数量受控。
//
// 背景（用户实测反馈）：把各导出动作挂到底栏快捷键上会被挤爆——
// 底部键位栏在与右侧状态争宽时会被截断（layout.go composeRule 的取舍顺序是
// 「状态不可丢、键位可压缩」），动作一多就被截掉，用户既看不到也按不到。
// 因此约定：动作在列表里（回车即可），底栏只留通用的「选择 / 导出 / 返回」。
func TestDumpScreenActionsAreListEntries(t *testing.T) {
	// 三类取数通道都必须有对应条目，否则某类数据就没有导出入口
	kinds := map[dumpKind]bool{}
	for _, opt := range dumpOptionList {
		kinds[opt.kind] = true
		if opt.title == "" {
			t.Fatalf("条目缺少标题: %#v", opt)
		}
		if opt.desc == "" {
			t.Fatalf("条目 %s 缺少说明", opt.title)
		}
		if opt.hotkey == "" {
			t.Fatalf("条目 %s 缺少热键（列表中应显示其键位）", opt.title)
		}
		if opt.kind == dumpKindContext && opt.key == "" {
			t.Fatalf("上下文条目 %s 缺少 agent 名", opt.title)
		}
	}
	for _, need := range []dumpKind{dumpKindLogs, dumpKindTranscript, dumpKindContext} {
		if !kinds[need] {
			t.Fatalf("缺少取数通道 %d 的条目", need)
		}
	}

	// 底栏不得膨胀：只保留通用键位，导出动作不在此枚举
	keys := dumpKeys(dumpState{})
	if len(keys) > 3 {
		t.Fatalf("底栏键位过多（%d 条），会与右侧状态争宽被截断: %#v", len(keys), keys)
	}
	if rendered := dispWidth(keyHints(keys)); rendered > 40 {
		t.Fatalf("底栏键位渲染宽度 %d 列，占位过大（窄终端会被截断）", rendered)
	}
	// 导出进行中时只提示状态，不给可能误导的键位
	if running := dumpKeys(dumpState{running: true}); len(running) != 1 {
		t.Fatalf("进行中应只提示状态，实际 %d 条", len(running))
	}
}

// 覆盖：热键按下标查找，且每个热键都能唯一定位到条目。
//
// 依 hotkey 字段解析（而非行号）的意义：今后在列表中间插入新条目，
// 既有热键的位置不会漂移——用户记住的键位仍然有效。
func TestDumpHotkeyIndexIsFieldBased(t *testing.T) {
	seen := map[string]int{}
	for i, opt := range dumpOptionList {
		if prev, dup := seen[opt.hotkey]; dup {
			t.Fatalf("热键 %q 重复：条目 %d 与 %d", opt.hotkey, prev, i)
		}
		seen[opt.hotkey] = i

		if got := dumpHotkeyIndex(opt.hotkey); got != i {
			t.Fatalf("热键 %q 定位错误: 期望 %d，实际 %d", opt.hotkey, i, got)
		}
	}

	// 未分配的编号、半截编号与空串必须返回 -1（调用方据此判断"不是完整热键"）
	for _, miss := range []string{"", "0", "1", "14", "20", "x", "esc", "enter"} {
		if got := dumpHotkeyIndex(miss); got != -1 {
			t.Fatalf("非热键 %q 不应命中，实际返回 %d", miss, got)
		}
	}

	// 回归防线：快捷键统一为两位数字编号（01-13），且必须指向固定目标。
	wantKeys := []struct {
		hotkey string
		kind   dumpKind
	}{
		{"01", dumpKindLogs},
		{"02", dumpKindTranscript},
		{"03", dumpKindContext},
		{"04", dumpKindContext},
		{"05", dumpKindContext},
		{"06", dumpKindContext},
		{"07", dumpKindContext},
		{"08", dumpKindContext},
		{"09", dumpKindContext},
		{"10", dumpKindContext},
		{"11", dumpKindContext},
		{"12", dumpKindContext},
		{"13", dumpKindContext},
	}
	for _, want := range wantKeys {
		got := dumpHotkeyIndex(want.hotkey)
		if got < 0 || dumpOptionList[got].kind != want.kind {
			t.Fatalf("编号 %q 未指向期望的导出类型", want.hotkey)
		}
	}
	if got := dumpHotkeyIndex("03"); dumpOptionList[got].key != "core" {
		t.Fatal("编号 03 应指向核心人格")
	}
	if got := dumpHotkeyIndex("04"); dumpOptionList[got].key != "file_editor" {
		t.Fatal("编号 04 应指向文件编辑")
	}
	if got := dumpHotkeyIndex("05"); dumpOptionList[got].key != "creative" {
		t.Fatal("编号 05 应指向自由创作")
	}
	if got := dumpHotkeyIndex("06"); dumpOptionList[got].key != "responder" {
		t.Fatal("编号 06 应指向通用应答")
	}
	if got := dumpHotkeyIndex("07"); dumpOptionList[got].key != "savior" {
		t.Fatal("编号 07 应指向智能修复")
	}
	if got := dumpHotkeyIndex("13"); dumpOptionList[got].key != "mutual_forum" {
		t.Fatal("编号 13 应指向互助论坛")
	}
}
