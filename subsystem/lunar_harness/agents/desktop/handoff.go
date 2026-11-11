//go:build windows

package desktop

// ==== 跨轮交接与尝试记录 ====
// HandoffRecord 是族成员之间唯一的信息介质；编排层额外维护目标应用与最近尝试，
// 让规划者能看到「哪些路已经走不通」，而不是每轮从零开始重复同一操作。

import (
	"fmt"
	"strings"

	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// BuildPlanContext 组装任务目标/验收标准/预期步骤，供各成员提示词复用。
func BuildPlanContext(plan EditorTask) string {
	return fmt.Sprintf("【任务目标】%s\n【验收标准】%s\n【预期步骤】%s",
		plan.Goal, strings.Join(plan.Acceptance, "；"), strings.Join(plan.StepsExpected, " → "))
}

// StringifyProgress 汇总当前进度（不含历史尝试）。
func StringifyProgress(h HandoffRecord) string {
	return fmt.Sprintf("目标=%s；前台=%s（进程 %s）；画面=%s；输入框=%s；动作=%s；进度=%s",
		h.TargetApp, h.Foreground, h.ForegroundProcess, textutil.TruncateRunes(h.VisualSummary, 512), h.InputBoxText, h.LastAction, h.Progress)
}

// StringifyHandoff 在进度之外追加最近尝试记录，供规划者避免重复无效操作。
func StringifyHandoff(h HandoffRecord) string {
	var sb strings.Builder
	sb.WriteString(StringifyProgress(h))
	if len(h.Attempts) == 0 {
		return sb.String()
	}
	sb.WriteString("\n【最近尝试】")
	for _, a := range h.Attempts {
		status := "成功"
		if !a.Success {
			status = "失败"
		}
		effect := "有变化"
		if !a.Changed {
			effect = "无变化"
		}
		tool := a.Tool
		if tool == "" {
			tool = "（未执行工具）"
		}
		fmt.Fprintf(&sb, "\n- 第%d轮 %s/%s：%s（%s，%s）",
			a.Round, a.Decision, tool, textutil.TruncateRunes(a.Result, 512), status, effect)
	}
	return sb.String()
}

// AppendAttempt 追加一次尝试并只保留最近 autoMaxAttempts 条。
func AppendAttempt(list []ActionAttempt, attempt ActionAttempt) []ActionAttempt {
	list = append(list, attempt)
	if len(list) > autoMaxAttempts {
		list = list[len(list)-autoMaxAttempts:]
	}
	return list
}

// AdvanceHandoff 由编排层确定性推进交接记录（替代原先依赖截图的「书记者」模型调用）：
// 状态只来自真实前台窗口与工具返回值，避免模型臆测出「桌面 / 记事本未打开」之类的错误画面描述。
func AdvanceHandoff(h HandoffRecord, attempt ActionAttempt) HandoffRecord {
	h.Round = attempt.Round
	h.Foreground = desktopos.ForegroundTitle()
	h.ForegroundProcess = desktopos.ForegroundProcess()
	h.LastAction = attempt.Result
	h.Evidence = attempt.Result
	switch {
	case !attempt.Success:
		h.Progress = "上一步未成功：" + textutil.TruncateRunes(attempt.Result, 512)
	case attempt.Changed:
		h.Progress = "上一步操作已生效"
	default:
		h.Progress = "上一步操作未产生可见变化"
	}
	h.VisualSummary = ""
	return h
}
