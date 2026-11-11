//go:build windows

package desktopchain

// verify.go 规划输出「无下一步」时的收尾核对（确定性，不调模型）。
//
// 判据只来自可查询的事实：目标应用是否在前台、当前 UIA 树里有没有最后一次
// 输入的文本、任务要求产出的路径是否存在。任何一项都拿不到确凿证据时一律
// Unknown——收尾好看不是证据，超时/取消更不是。
//
// 实况里本核对登记为一对「调用 verify_task / 结果 三态」：
// 该动作由本链真实派发（前台查询 + 窗口枚举 + UIA 树读取 + 路径探测），
// 不是模型工具调用，也不会额外冒出没有调用行的结果。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

const (
	// verifyToolName 收尾核对在实况里的动作名（链级确定性核对，非模型工具）。
	verifyToolName = "verify_task"
	// verifyTimeout 单次核对的总时限。
	verifyTimeout = 6 * time.Second
)

var (
	// producePathRe 提取「创建/新建/保存/写入/生成/导出/另存 … <绝对路径>」里的产出路径。
	producePathRe = regexp.MustCompile(`(?:创建|新建|保存|写入|生成|导出|另存)[^A-Za-z]{0,8}([A-Za-z]:\\[^\s"'「」《》，,；;]+)`)
	// openIntentRe 判断任务是否为「打开/启动/激活某个应用」。
	openIntentRe = regexp.MustCompile(`打开|启动|激活|切换到|切到`)
)

// verifyFacts 收尾核对采集到的事实（全部可复核，不含模型判断）。
type verifyFacts struct {
	foreground       string // 当前前台窗口标题
	targetApp        string // 本轮目标应用关键字
	targetForeground bool   // 目标应用是否在前台
	targetWindow     bool   // 是否存在匹配目标应用的可见窗口
	lastInput        string // 最近一次成功提交的文本输入
	uiaOK            bool   // UIA 树是否读取成功
	uiaTree          string // UIA 树文本
	uiaErr           string // UIA 读取失败原因（成功时为空）
	producePath      string // 任务要求产出的绝对路径
	produceIntent    bool   // 任务是否明确要求创建/保存到该路径
	pathExists       bool   // 该路径是否已存在
	openIntent       bool   // 任务是否为打开/启动类
	canceled         bool   // 核对期间被取消
}

// verifyOutcome 执行一次收尾核对，并把「调用 + 结果」成对写进实况。
func verifyOutcome(ctx context.Context, plan desktop.EditorTask, handoff desktop.HandoffRecord) (agentbase.ToolResultState, string) {
	lastInput := lastInputText(handoff)
	args, _ := json.Marshal(map[string]any{
		"target_app": handoff.TargetApp,
		"last_input": clipText(lastInput, 32),
	})

	runCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	facts := collectVerifyFacts(runCtx, plan, handoff, lastInput)
	if runCtx.Err() != nil {
		facts.canceled = true
	}

	state, summary := judgeVerify(facts)
	reportVerify(string(args), state, summary)
	logger.Info("LunarHarness", chainScopeLabel+"收尾核对：%s（%s）", state.Label(), summary)
	return state, summary
}

// reportVerify 把一次收尾核对登记为成对的「调用 + 结果」实况行。
func reportVerify(args string, state agentbase.ToolResultState, summary string) {
	agentbase.ReportToolCall(chainScopeLabel, verifyToolName, args)
	agentbase.ReportToolResult(chainScopeLabel, verifyToolName, state, summary)
}

// collectVerifyFacts 采集前台、窗口、UIA 与产出路径四类事实。
func collectVerifyFacts(ctx context.Context, plan desktop.EditorTask, handoff desktop.HandoffRecord, lastInput string) verifyFacts {
	text := planText(plan)
	facts := verifyFacts{
		foreground: desktopos.ForegroundTitle(),
		targetApp:  strings.TrimSpace(handoff.TargetApp),
		lastInput:  lastInput,
		openIntent: openIntentRe.MatchString(text),
	}
	facts.producePath = declaredProducePath(text)
	if facts.producePath != "" {
		facts.produceIntent = true
		if _, err := os.Stat(facts.producePath); err == nil {
			facts.pathExists = true
		}
	}
	if facts.targetApp != "" {
		facts.targetForeground = desktopos.ForegroundMatches(facts.targetApp)
		key := strings.ToLower(facts.targetApp)
		for _, w := range desktopos.ListWindows() {
			if strings.Contains(strings.ToLower(w.Title), key) || strings.Contains(strings.ToLower(w.Process), key) {
				facts.targetWindow = true
				break
			}
		}
	}
	if lastInput != "" {
		tree, err := desktop.CallDesktop(ctx, desktop.UIADumpTimeout, "读取 UIA 树", func() (string, error) {
			dump, ok, dumpErr := desktopos.UIADump()
			if dumpErr != nil {
				return "", dumpErr
			}
			if !ok {
				return "", errors.New("UIA 树读取失败")
			}
			return dump, nil
		})
		if err != nil {
			if errors.Is(err, desktop.ErrCanceled) {
				facts.canceled = true
			}
			facts.uiaErr = err.Error()
		} else {
			facts.uiaOK = true
			facts.uiaTree = tree
		}
	}
	return facts
}

// judgeVerify 依据采集到的事实判定三态：
//   - NotApplied：任务明确要求产出的路径不存在（唯一可确定「未达成」的证据）；
//   - Applied：产出路径已存在、界面已包含最后输入的文本、或打开类任务的目标应用已在前台；
//   - Unknown：其余全部情况（取消、读取失败、找不到文本、窗口不在前台、无证据）。
func judgeVerify(f verifyFacts) (agentbase.ToolResultState, string) {
	if f.canceled {
		return agentbase.ToolUnknown, "核对已取消，状态未确认"
	}
	if f.produceIntent && f.producePath != "" {
		if f.pathExists {
			return agentbase.ToolApplied, "任务要求产出的路径已存在：" + f.producePath
		}
		return agentbase.ToolNotApplied, "任务要求产出 " + f.producePath + "，但该路径不存在"
	}
	if f.lastInput != "" {
		if f.uiaOK && strings.Contains(f.uiaTree, f.lastInput) {
			return agentbase.ToolApplied, "当前界面元素已包含最后输入的文本：" + clipText(f.lastInput, 32)
		}
		if f.uiaErr != "" {
			return agentbase.ToolUnknown, "界面元素读取失败（" + f.uiaErr + "），输入结果未确认"
		}
		return agentbase.ToolUnknown, "界面元素里暂未找到最后输入的文本，无法确认是否生效"
	}
	if f.openIntent && f.targetApp != "" {
		if f.targetForeground {
			return agentbase.ToolApplied, "目标应用已在前台：" + f.foreground
		}
		if f.targetWindow {
			return agentbase.ToolUnknown, "目标应用窗口存在但不在前台：" + f.foreground
		}
		return agentbase.ToolUnknown, "未发现目标应用窗口，无法确认任务是完成还是已被关闭"
	}
	return agentbase.ToolUnknown, "没有可核对的确定性证据（前台窗口 / 界面文本 / 产出路径），状态未确认"
}

// closingText 把核对结果写进最终回执，避免「没有下一步」被读成「已完成」。
func closingText(state agentbase.ToolResultState, summary, reason string) string {
	switch state {
	case agentbase.ToolApplied:
		return "任务完成：" + summary + "（规划侧没有下一步：" + reason + "）"
	case agentbase.ToolNotApplied:
		return "任务未完成：" + summary + "（规划侧没有下一步：" + reason + "）"
	default:
		return "任务状态未确认：" + summary + "（规划侧没有下一步：" + reason + "）"
	}
}

// planText 汇总编纂结果里可用于判定的文本。
func planText(plan desktop.EditorTask) string {
	parts := []string{plan.Task, plan.Goal}
	parts = append(parts, plan.Acceptance...)
	parts = append(parts, plan.StepsExpected...)
	return strings.Join(parts, "\n")
}

// declaredProducePath 提取任务明确要求产出的绝对路径；没有明确产出意图时返回空串。
func declaredProducePath(text string) string {
	m := producePathRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimRight(m[1], `.。!！?？)`+"」")
}

// lastInputText 取最近一次成功提交的文本输入（可能不在最后一轮动作里）。
func lastInputText(handoff desktop.HandoffRecord) string {
	for i := len(handoff.Attempts) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(handoff.Attempts[i].InputText); text != "" {
			return text
		}
	}
	return ""
}

// clipText 截断过长文本，避免实况行被整段输入撑爆。
func clipText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
