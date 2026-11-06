package thinking

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// TestDialogueRegisteredAsRepairTarget 证明对话者的修复目标已进入拯救者可见清单。
func TestDialogueRegisteredAsRepairTarget(t *testing.T) {
	registered := false
	for _, target := range agentrepair.List() {
		if target.ID != dialogueAgentID {
			continue
		}
		registered = true
		if target.Label == "" || target.Description == "" {
			t.Fatalf("对话者修复目标缺少名称或描述: %#v", target)
		}
		actions := map[string]bool{"trim": false, "drop_images": false, "clear": false}
		for _, action := range target.Actions {
			if _, ok := actions[action]; ok {
				actions[action] = true
			}
		}
		for action, found := range actions {
			if !found {
				t.Fatalf("对话者修复目标缺少动作 %s（实得 %v）", action, target.Actions)
			}
		}
	}
	if !registered {
		t.Fatal("agentrepair.List 中缺少 dialogue（误导者无法定位到对话者上下文）")
	}
	// 按 ID 能命中处理器：非法动作由处理器自身拒绝，说明 lookup 成功且不会产生副作用。
	if _, err := agentrepair.Repair(dialogueAgentID, "no_such_action", nil); err == nil || !strings.Contains(err.Error(), "不支持动作") {
		t.Fatalf("dialogue 修复入口不可用: %v", err)
	}
}

// TestDialogueFailureProducesRepairEntry 证明一次失败上报会在拯救者队列里生成可修复条目。
func TestDialogueFailureProducesRepairEntry(t *testing.T) {
	agentbase.SetRepairHook(Savior.ReportRun)
	t.Cleanup(func() { agentbase.SetRepairHook(nil) })

	marker := fmt.Sprintf("对话者模型调用失败（注册测试 %d）", time.Now().UnixNano())
	reportDialogueFailure(errors.New(marker))

	if Savior.Pending() == 0 {
		t.Fatal("故障没有进入拯救者队列")
	}
	raw, err := agentdump.Export("savior", false)
	if err != nil {
		t.Fatalf("导出拯救者故障队列失败: %v", err)
	}
	var payload struct {
		Failures []struct {
			Agent string `json:"agent"`
			Error string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("解析故障队列失败: %v（原文 %s）", err, raw)
	}
	for _, failure := range payload.Failures {
		if failure.Agent == dialogueAgentID && strings.Contains(failure.Error, marker) {
			return
		}
	}
	t.Fatalf("故障队列里没有 dialogue 条目（原文 %s）", raw)
}

// TestTickErrorReportsThroughReportRun 证明 Tick 调度失败走 ReportRun：
// 携带调度阶段信息，并过滤主动取消/循环截断这类非上下文故障。
func TestTickErrorReportsThroughReportRun(t *testing.T) {
	reports := captureReports(t)

	reportTickError("dialogue", errors.New("tick boom"))
	got := reports()
	if len(got) != 1 {
		t.Fatalf("应上报 1 条 Tick 失败，实得 %d 条", len(got))
	}
	if got[0].agent != dialogueAgentID || !strings.Contains(got[0].err.Error(), "Tick 调度失败（dialogue）") {
		t.Fatalf("上报内容不正确: %#v", got[0])
	}

	// ReportRun 语义：循环截断被过滤；若仍用 Report 则会多出一条。
	reportTickError("dialogue", agentbase.ErrLoop)
	reportTickError("dialogue", nil)
	if got := reports(); len(got) != 1 {
		t.Fatalf("ReportRun 未过滤循环截断/空错误，实得 %d 条", len(got))
	}

	// Tick 体系映射仍然生效：非对话体系按自身 ID 上报。
	reportTickError("life", errors.New("life boom"))
	got = reports()
	if len(got) != 2 || got[1].agent != "life" {
		t.Fatalf("life 应按自身 ID 上报，实得 %#v", got)
	}
}
