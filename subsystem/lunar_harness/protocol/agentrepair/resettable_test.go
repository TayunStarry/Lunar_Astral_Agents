package agentrepair

// resettable_test.go 钉住「注册 resettable 但未注册 dump」的语义：
// 显式 inspect 优先；未提供 inspect 的旧调用方退回 agentdump 查找，
// 查不到时目标仍可列出（明确降级），Inspect 返回带原因的错误。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func TestRegisterResettableWithExplicitInspect(t *testing.T) {
	const id = "test_resettable_explicit"
	RegisterResettable(id, "测试目标", "显式 inspect", func() string { return "已重置" },
		func(limit int) (any, error) {
			return map[string]any{"kind": "explicit", "limit": limit}, nil
		})

	if !listed(id) {
		t.Fatalf("显式注册后 %s 应出现在修复清单", id)
	}
	out, err := Inspect(id, 0)
	if err != nil {
		t.Fatalf("显式 inspect 不应依赖 dump: %v", err)
	}
	if !strings.Contains(out, `"kind": "explicit"`) {
		t.Fatalf("Inspector 输出异常: %s", out)
	}
	if msg, rerr := Repair(id, actionReset, nil); rerr != nil || msg != "已重置" {
		t.Fatalf("reset 失败: msg=%q err=%v", msg, rerr)
	}
}

func TestRegisterResettableWithoutDumpDegradesExplicitly(t *testing.T) {
	const id = "test_resettable_no_dump"
	RegisterResettable(id, "测试目标", "未注册 dump", func() string { return "已重置" })

	// 明确降级语义：目标仍可列出，拯救者能感知它的存在。
	if !listed(id) {
		t.Fatalf("未注册 dump 时 %s 仍应出现在修复清单（明确降级）", id)
	}
	// 但读取上下文必须明确报错，而不是返回空内容或静默成功。
	_, err := Inspect(id, 0)
	if err == nil {
		t.Fatal("未注册 dump 时 Inspect 应返回错误")
	}
	if !strings.Contains(err.Error(), "未注册同名上下文导出") {
		t.Fatalf("错误信息应指出 dump 缺失: %v", err)
	}
	// reset 不依赖上下文，仍可执行。
	if msg, rerr := Repair(id, actionReset, nil); rerr != nil || msg != "已重置" {
		t.Fatalf("未注册 dump 不应影响 reset: msg=%q err=%v", msg, rerr)
	}
}

func TestRegisterResettableLegacyWithDumpKeepsBehavior(t *testing.T) {
	const id = "test_resettable_legacy_dump"
	agentdump.Register(agentdump.Provider{
		ID: id,
		Export: func(bool) (any, error) {
			return map[string]any{"kind": "legacy"}, nil
		},
	})
	// 旧式调用：不传 inspect，按 ID 查 dump（保留现有调用方行为）。
	RegisterResettable(id, "测试目标", "旧式隐式查找", func() string { return "已重置" })

	out, err := Inspect(id, 0)
	if err != nil {
		t.Fatalf("已注册同名 dump 时旧式查找应成功: %v", err)
	}
	if !strings.Contains(out, `"kind": "legacy"`) {
		t.Fatalf("旧式查找输出异常: %s", out)
	}
}

func listed(id string) bool {
	for _, info := range List() {
		if info.ID == id {
			return true
		}
	}
	return false
}
