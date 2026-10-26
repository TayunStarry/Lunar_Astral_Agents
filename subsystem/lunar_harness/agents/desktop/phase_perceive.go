//go:build windows

package desktop

// ==== 感知阶段：UIA 理解者 ====
// 视觉摘要已移除：每轮 10-20 秒的画面描述经常与 UIA/前台状态矛盾，反而误导规划。
// 截图只在真正需要坐标的鼠标操作时直接交给鼠标操作者（见 phase_operate.go）。

import (
	"context"
)

// uiaTreeResult UIA 树读取结果（用于带超时调用）
type uiaTreeResult struct {
	tree string
	ok   bool
}

// uiaReaderPhase UIA 理解者：读取前台窗口 UI 树并让模型整理与任务相关的元素清单。
func uiaReaderPhase(ctx context.Context, plan editorTask) (string, []string) {
	result, err := callDesktop(ctx, uiaDumpTimeout, "读取 UIA 树", func() (uiaTreeResult, error) {
		tree, ok, dumpErr := DTUIADump()
		return uiaTreeResult{tree: tree, ok: ok}, dumpErr
	})
	if err != nil || !result.ok {
		return "（UIA 树读取失败）", nil
	}
	tree := result.tree
	elements := []string{}
	msg := chatMessage{Role: "user", Content: buildPlanContext(plan) + "\n\n【UI 树】\n" + truncateStr(tree, 4096) + "\n\n请整理与任务相关的元素清单与摘要 JSON。"}
	text, err := autoChatText(ctx, autoRoleUIA, []chatMessage{{Role: "system", Content: promptUIAReader}, msg})
	if err != nil {
		elements = append(elements, "(UIA 理解失败)")
	} else {
		elements = append(elements, text)
	}
	return truncateStr(tree, 400), elements
}
