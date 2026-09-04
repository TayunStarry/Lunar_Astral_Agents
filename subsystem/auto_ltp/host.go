//go:build windows

package AutoLTP

// ==== 宿主循环调度器 ====
// 编排 Auto-LTP 全流程：初始构建（编者→启动者）+ 执行循环
//（视觉→UIA→规划→仅一个操作者→进度书记→下一轮），
// 每个角色独立全新上下文，工具白名单物理隔离。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Run 运行 Auto-LTP，处理一条用户指令，返回结果文本。
func Run(instruction string) (resultText string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Auto-LTP 内部异常: %v", r)
		}
	}()

	// —— 阶段 1：提示词编纂者（无工具） ——
	plan, err := editorPhase(instruction)
	if err != nil {
		return "", err
	}
	var handoff HandoffRecord

	// —— 阶段 2：软件启动者 ——
	if err := launcherPhase(plan, &handoff); err != nil {
		return "", fmt.Errorf("软件启动失败: %v", err)
	}

	// —— 阶段 3：执行循环 ——
	maxLoops := 30
	for round := 1; round <= maxLoops; round++ {
		handoff.Round = round

		// 3.1 视觉理解者（无工具，读自动注入截图）
		handoff.VisualSummary = visionPhase(plan, handoff)
		// 3.2 UIA 理解者（读 UI 树/定向查）
		treeSummary, elements := uiaReaderPhase(plan)
		// 3.3 任务规划者（无工具，决策）
		decision := plannerPhase(plan, handoff, treeSummary, elements)

		if decision.Decision == "complete" {
			return decision.Action, nil
		}

		// 3.4 仅启用一个操作者
		performed := operationPhase(decision)
		handoff.LastAction = performed

		// 3.5 进度书记者（截图+记录）
		handoff = scribePhase(plan, handoff)
	}
	return "已达到最大执行轮次，任务暂停", nil
}

// editorPhase 提示词编纂者：将用户原始指令解析为结构化任务 JSON。
func editorPhase(instruction string) (editorTask, error) {
	msg := chatMessage{Role: "user", Content: "用户请求：「" + instruction + "」\n\n请输出结构化任务 JSON。"}
	text, err := autoChatText([]chatMessage{{Role: "system", Content: promptEditor}, msg})
	if err != nil {
		return editorTask{}, err
	}
	var t editorTask
	if err := json.Unmarshal(extractJSON(text), &t); err != nil {
		// 兜底：解析失败也保留原始指令，不阻塞流程
		return editorTask{Task: instruction, Goal: instruction, Acceptance: []string{}}, nil
	}
	return t, nil
}

// launcherPhase 软件启动者：查找/打开目标应用，最多进行 3 轮工具调用。
func launcherPhase(plan editorTask, handoff *HandoffRecord) error {
	sysReq := chatMessage{Role: "user", Content: buildPlanContext(plan) + "\n\n请按流程找到/打开目标应用并输出 JSON。"}
	finalTools := roleTools(launcherTools)
	messages := []chatMessage{{Role: "system", Content: promptLauncher}, sysReq}
	// 最多 3 轮工具调用
	for range 3 {
		msg, err := autoChat(messages, finalTools)
		if err != nil {
			return err
		}
		if len(msg.ToolCalls) == 0 {
			handoff.Foreground = DTForegroundTitle()
			return nil
		}
		tc := msg.ToolCalls[0]
		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: []ltpToolCall{tc}})
		args := parseLTPArgs(tc.Function.Arguments)
		res := dispatchWithDiff(tc.Function.Name, args)
		// 通用「启动即置前台」：启动/打开应用后立刻把新窗口提到前台，避免“只能看到旧窗口/开始界面”的体感
		if res.Success && (tc.Function.Name == "launch_program" || tc.Function.Name == "open_folder") {
			key := ltpArgStr(args, "name")
			if key == "" {
				if p := ltpArgStr(args, "path"); p != "" {
					key = filepath.Base(strings.TrimSpace(strings.TrimRight(p, `\`)))
				}
			}
			if t := DTActivateLaunchByName(key); t != "" {
				traceAuto("【启动者】启动即置前台: %s", t)
			}
		}
		rb, _ := json.Marshal(res)
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
	}
	return nil
}

// visionPhase 视觉理解者：把当前截图注入，让模型摘要画面，返回画面摘要文本。
func visionPhase(plan editorTask, handoff HandoffRecord) string {
	parts := []contentPart{{Type: "text", Text: buildPlanContext(plan) + "\n\n【上一轮进度】" + stringifyHandoff(handoff) + "\n\n请基于下方注入的画面截图，摘要与任务相关的信息。"}}
	parts[0].Text += "\n<image>"
	if img, err := DTCaptureAnnotated(); err == nil {
		importB64 := "data:image/png;base64," + base64Encode(img)
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: importB64}})
	}
	msg := chatMessage{Role: "user", Content: parts}
	text, err := autoChatText([]chatMessage{{Role: "system", Content: promptVision}, msg})
	if err != nil {
		return "（视觉理解失败）"
	}
	return text
}

// uiaReaderPhase UIA 理解者：读取前台窗口 UI 树并让模型整理与任务相关的元素清单。
func uiaReaderPhase(plan editorTask) (string, []string) {
	tree, ok, err := DTUIADump()
	if !ok || err != nil {
		return "（UIA 树读取失败）", nil
	}
	elements := []string{}
	msg := chatMessage{Role: "user", Content: buildPlanContext(plan) + "\n\n【UI 树】\n" + truncateStr(tree, 4096) + "\n\n请整理与任务相关的元素清单与摘要 JSON。"}
	text, err := autoChatText([]chatMessage{{Role: "system", Content: promptUIAReader}, msg})
	if err != nil {
		elements = append(elements, "(UIA 理解失败)")
	} else {
		elements = append(elements, text)
	}
	return truncateStr(tree, 400), elements
}

// plannerPhase 任务规划者：综合视觉/UIA 情报决策下一步，返回决策结构。
func plannerPhase(plan editorTask, handoff HandoffRecord, treeSummary string, elements []string) planDecision {
	var sb strings.Builder
	sb.WriteString(buildPlanContext(plan))
	sb.WriteString("\n")
	sb.WriteString("【进度】")
	sb.WriteString(stringifyHandoff(handoff))
	sb.WriteString("\n")
	sb.WriteString("【视觉情报】")
	sb.WriteString(handoff.VisualSummary)
	sb.WriteString("\n")
	sb.WriteString("【UIA 情报】树摘要:")
	sb.WriteString(treeSummary)
	sb.WriteString("\n元素:")
	sb.WriteString(strings.Join(elements, "；"))
	sb.WriteString("\n")
	sb.WriteString("请决策下一步（mouse/keyboard/uia/complete），输出 JSON。")
	text, err := autoChatText([]chatMessage{{Role: "system", Content: promptPlanner}, {Role: "user", Content: sb.String()}})
	if err != nil {
		return planDecision{Decision: "complete", Action: "（规划失败，结束）"}
	}
	var d planDecision
	if json.Unmarshal(extractJSON(text), &d) != nil {
		return planDecision{Decision: "complete", Action: "（规划输出无法解析，结束）"}
	}
	return d
}

// operationPhase 执行操作者（键盘/鼠标/UIA）：按决策调用对应操作工具，返回执行描述。
func operationPhase(dec planDecision) string {
	var tools []string
	switch dec.Decision {
	case "keyboard":
		tools = keyboardTools
	case "mouse":
		tools = mouseTools
	case "uia":
		tools = uiaOpTools
	default:
		return dec.Action
	}
	var prompt string
	switch dec.Decision {
	case "keyboard":
		prompt = promptKeyboard
	case "mouse":
		prompt = promptMouse
	case "uia":
		prompt = promptUIAOp
	}
	messages := []chatMessage{{Role: "system", Content: prompt}, {Role: "user", Content: "任务目标：" + dec.Action}}
	for i := 0; i < 3; i++ {
		msg, err := autoChat(messages, roleTools(tools))
		if err != nil {
			return "操作失败: " + err.Error()
		}
		if len(msg.ToolCalls) == 0 {
			return messageText(msg.Content)
		}
		tc := msg.ToolCalls[0]
		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: []ltpToolCall{tc}})
		args := parseLTPArgs(tc.Function.Arguments)
		res := dispatchWithDiff(tc.Function.Name, args)
		rb, _ := json.Marshal(res)
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
		if res.Success {
			return fmt.Sprintf("%s(%s)", dec.Decision, res.Text)
		}
	}
	return "操作未在限定轮内成功"
}

// scribePhase 进度书记者：核对真实状态并沉淀新的交接记录，供下一轮全新上下文仅依赖它推进。
func scribePhase(plan editorTask, handoff HandoffRecord) HandoffRecord {
	h := handoff
	h.Foreground = DTForegroundTitle()
	var sb strings.Builder
	sb.WriteString(buildPlanContext(plan))
	sb.WriteString("\n")
	sb.WriteString("【进度到本轮】")
	sb.WriteString(stringifyHandoff(h))
	sb.WriteString("\n")
	sb.WriteString("本轮已执行动作：")
	sb.WriteString(h.LastAction)
	sb.WriteString("。请核对真实状态并输出严格 JSON。")
	// 注入截图
	parts := []contentPart{{Type: "text", Text: sb.String()}}
	parts[0].Text += "\n<image>"
	if img, err := DTCaptureAnnotated(); err == nil {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: "data:image/png;base64," + base64Encode(img)}})
	}
	msg := chatMessage{Role: "user", Content: parts}
	text, err := autoChatText([]chatMessage{{Role: "system", Content: promptScribe}, msg})
	if err != nil {
		return h
	}
	var nr HandoffRecord
	if json.Unmarshal(extractJSON(text), &nr) != nil {
		return h
	}
	nr.Round = h.Round
	return nr
}

// ==== 工具函数 ====

// buildPlanContext 组装任务目标/验收标准/预期步骤，供各角色提示词复用。
func buildPlanContext(plan editorTask) string {
	return fmt.Sprintf("【任务目标】%s\n【验收标准】%s\n【预期步骤】%s",
		plan.Goal, strings.Join(plan.Acceptance, "；"), strings.Join(plan.StepsExpected, " → "))
}

// stringifyHandoff 将交接记录汇总为单行字符串，便于注入提示词。
func stringifyHandoff(h HandoffRecord) string {
	return fmt.Sprintf("前台=%s；画面=%s；输入框=%s；动作=%s；进度=%s",
		h.Foreground, truncateStr(h.VisualSummary, 120), h.InputBoxText, h.LastAction, h.Progress)
}

// extractJSON 从模型输出文本中提取第一组大括号包裹的 JSON 片段。
func extractJSON(s string) []byte {
	clean := strings.TrimSpace(s)
	start := strings.IndexByte(clean, '{')
	end := strings.LastIndexByte(clean, '}')
	if start < 0 || end <= start {
		return []byte(s)
	}
	return []byte(clean[start : end+1])
}

// base64Encode 将字节数据编码为标准 base64 字符串（用于内嵌图片）。
func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
