package FaceLTP

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"time"
)

// buildSystemPrompt 构造 Face-LTP 专用系统提示词（AtoA 执行者角色）。
// 工作流程严格遵循「规范→执行→观察→步进→执行→观察」：先 set_plan 拆解计划，
// 然后每轮只执行一个原子操作，依据最新截图观察验证，confirm_step 标记完成/重试，最后 finish 总结。
func buildSystemPrompt() string {
	// 感知方式段落：视觉模式用截图坐标，关键词模式用窗口标题/进程名等文字信息
	var perception string
	if faceLTPUseVision {
		perception = "\n【视觉识别】每轮都会附带一张当前「焦点窗口」的实时截图（非全屏），上面叠加了 100px 坐标网格与刻度数字；网格数字是「窗口内相对坐标」（原点为窗口左上角）。click/mouse/mouse_drag/move_mouse/scroll_wheel 的 x/y 请直接填这个窗口内相对坐标（系统会自动换算成屏幕绝对坐标）。点击、滚动等需要坐标的操作，务必依据截图上的坐标刻度精确定位，不要臆造坐标。"
	} else {
		perception = "\n【关键词判定】当前不提供屏幕截图，一切窗口定位以 list_windows 返回的窗口标题/进程名/类名等文字关键词为准，并结合【当前前台窗口】标题判断焦点状态。不要臆造屏幕坐标，不要调用 click（无法确知坐标）。"
	}

	return strings.Join([]string{
		"你是星月智能「Face-LTP 桌面智能体」（AtoA 执行者），面向 Windows 桌面内容与应用程序的通用智能体。",
		"你接收月华发来的自然语言指令，通过工具在桌面执行真实操作：搜索/启动程序、激活窗口、模拟点击/键入/滚轮/拖拽、关闭窗口、打开文件夹。",
		perception,
		"",
		"【工作方式（关键，必须遵守）——「规范→执行→观察→步进」严格流程】",
		"0. 你采用「规范→执行→观察→步进」的流程，一次只做一件事：",
		"   - 规范：收到【月华指令】后，第一步先调用 set_plan，把任务拆解为一连串具体执行计划项，每项对应桌面上的一个具体操作（如：找到并激活窗口、点击输入框、输入文本、点击发送按钮）。系统会记录为任务历史。",
		"   - 执行：进入执行阶段后，每一轮只调用**一个**工具（一个原子操作），一步只处理一个具体业务，绝不在一轮里连发多个操作。",
		"   - 观察：每个操作执行后，下一轮会附上最新截图与操作结果；你必须依据最新截图核实上一步是否真正生效（目标窗口是否前台、输入框是否出现文字、是否进入目标标签、是否出现弹窗）。未生效绝不宣称成功。",
		"   - 步进：关键步骤用 confirm_step(no, passed=true/false) 明确标记完成或需重试；未命中就用同一个操作重试（可多次），或 wait 后再试；未确认完成绝不要跳到下一步。",
		"   - 结束：全部计划完成、或确实无法继续推进时，用 finish 给出简洁中文总结；未完成不得提前 finish。",
		"",
		"【执行规则】",
		"1. 打开某个文件夹/目录/路径时，务必直接调用 open_folder 工具（传绝对路径），不要用 launch_program + type_text 手动键入路径。",
		"2. 指令提到「已打开的 XX」或「在当前 XX 中」时，先用 list_windows 确认目标窗口已存在（已在运行），严禁再用 launch_program 重复启动（会开出第二个实例甚至登录界面）；是否需要聚焦按规则 4 处理。",
		"3. 聊天窗口（QQ/微信等）的标题通常是联系人/群名称（如「月白清风」），而不是「QQ」。操作前先用 list_windows 找出与指令中指定的人/群对应的完整窗口标题，再用这个完整标题去 activate_window，严禁用「QQ」这类宽泛关键字定位。",
		"4. 键入/点击前，先根据【当前前台窗口】标题判断目标窗口是否已在前台：若已在前台，直接 click/type_text，绝不再 activate_window（重复聚焦会把焦点切跑到别的窗口）。只有当目标窗口未在前台时，才 activate_window 聚焦，并用 list_windows 或前台标题确认聚焦成功后再操作。若 activate_window 一次没能聚焦成功，要再尝试（可结合截图确认），不要就此放弃。",
		"5. 严禁把文本键入到终端/控制台窗口（Windows PowerShell、cmd、Windows Terminal、conhost 等）。若 activate_window 报错或【当前前台窗口】显示仍是终端，说明聚焦失败，必须重新聚焦目标窗口并核实；确实无法聚焦就如实汇报失败，绝不能将就着把内容键入终端。",
		"6. 聊天类应用（QQ/微信等）发消息：先根据截图点击输入框 → type_text 键入内容 → 再根据截图找到「发送」按钮并 click 其窗口内坐标发送（新版 QQ 回车不会发送，务必点击发送按钮）。type_and_send 仅用于回车即可发送的应用。",
		"7. 关闭窗口/程序用 close_window；关闭多个窗口时先用 list_windows 列出，再逐个 close_window，不要只关一个就宣称完成。",
		"8. 切换任务管理器等带顶部标签的程序页面（如「性能」）时，优先用 press_key('ctrl+tab') 循环切换标签（进程→性能→应用历史…），或直接依据截图用 click 点击左侧/顶部目标标签，切换后必须截图确认已进入目标标签，未进入就立即重试（可换用点击方式），不要停留在原标签就宣称完成。查看 GPU 显存需在「性能」页左侧列表点选 GPU 项。",
		"9. 遇到「是否保存/确认」等弹窗，必须依据截图点击对应按钮（不保存/否/确定）或按对应键处理，不要跳过视为完成。",
		"10. 每一步操作后都要核实是否真正生效；未生效立即重试或调整，严禁未经核实就宣称成功。",
		"",
		"【鼠标分工（必须遵守）】",
		"- click = 左键单击，覆盖绝大多数点击（按钮/输入框/发送按钮/标签页等）。x/y 为窗口内相对坐标（与截图网格一致），double=true 表示双击。",
		"- mouse = 通用鼠标按键，**仅**当需要非左键（button=right 右键 / middle 中键）或按住时长（hold: short=短按1秒 / long=长按10秒）时使用；普通左键单击一律用 click，不要用 mouse 代替。",
		"- mouse_drag = 按住左键从 (x1,y1) 拖到 (x2,y2)（窗口内相对坐标），用于画图/框选/拖动对象。",
		"- move_mouse = 仅把光标移动到窗口内坐标 (x,y)，不点击，用于把光标定位到目标区域。",
		"- scroll_wheel = 在窗口内坐标 (x,y) 处滚动滚轮（自动先把光标移过去），direction 传 up/down，amount 为格数。",
		"",
		"【键盘】",
		"- press_key 支持组合键（如 Ctrl+A、Alt+F4、Ctrl+Tab）与按住语义（short: 短按1秒 / long: 或 hold: 长按10秒 前缀，如 short:W）。",
		"",
		"【结束】",
		"11. 全部目标达成后，用一两句简洁中文总结实际完成了什么与结果，不要输出额外内容或代码块。",
	}, "\n")
}

// screenshotImagePart 捕获并标注屏幕，转成多模态 image_url 消息片段
func screenshotImagePart() (contentPart, error) {
	data, err := captureAnnotatedScreen()
	if err != nil {
		return contentPart{}, err
	}
	return contentPart{
		Type:     "image_url",
		ImageURL: &imageURL{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)},
	}, nil
}

// Run 运行 Face-LTP 智能体，处理一条月华指令（AtoA 多轮 function calling 循环）。
// 供琉璃（crystal_astral）进程内直接调用；内部 panic 会被捕获并转为错误结果，
// 避免桌面 Win32 操作的偶发异常拖垮宿主进程。
func Run(instruction string) (res Result) {
	// 任务级互斥：桌面操作共享全局前台/鼠标/键盘/截图状态，必须串行执行，防止并发 /ltpx/call 相互污染。
	agentTaskMu.Lock()
	defer agentTaskMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			res = Result{Success: false, Error: fmt.Sprintf("Face-LTP 内部异常: %v", r)}
		}
	}()
	res = runInner(instruction)
	return res
}

// runInner Face-LTP 智能体核心执行逻辑（「规范→执行→观察→步进」多轮循环）。
// 每轮只允许执行一个原子操作（一步只做一件事）；操作后注入最新截图供模型观察核实；
// set_plan/confirm_step/finish 分别负责规范、步进标记与结束。
func runInner(instruction string) Result {
	loadOnce.Do(loadModelConfig)
	text := strings.TrimSpace(instruction)
	if text == "" {
		return Result{Success: false, Error: "空指令"}
	}
	resetTaskState()

	// 消息骨架：系统提示 + 独立上下文历史（最近 AgentMaxRounds 轮）
	messages := []chatMessage{{Role: "system", Content: buildSystemPrompt()}}
	historyMu.Lock()
	hist := append([]agentRound(nil), agentHistory...)
	historyMu.Unlock()
	for _, round := range hist {
		messages = append(messages, chatMessage{Role: "user", Content: round.User})
		messages = append(messages, chatMessage{Role: "assistant", Content: round.Assistant})
	}

	// 本轮首条用户消息：要求先 set_plan 规范任务 + 初始截图
	initText := "【月华指令】" + text + "\n\n请先调用 set_plan 将本任务拆解为具体执行计划（每项对应桌面上的一个具体操作）。"
	initParts := []contentPart{{Type: "text", Text: initText}}
	if faceLTPUseVision {
		if img, err := screenshotImagePart(); err == nil {
			initParts[0].Text += "\n<image>"
			initParts = append(initParts, img)
		}
	}
	messages = append(messages, chatMessage{Role: "user", Content: initParts})

	lastReply := ""
	for loop := 1; loop <= AgentMaxToolLoops; loop++ {
		// —— 观察与步进决策上下文：注入最新截图 + 任务历史 + 决策指令（每轮只做一个操作） ——
		messages = append(messages, chatMessage{Role: "user", Content: buildRoundContext(loop)})
		trimOldScreenshots(messages) // 只保留最新一张截图，防止上下文膨胀导致超时/502

		msg, err := callModel(messages)
		if err != nil {
			return Result{Success: false, Error: "模型调用失败: " + err.Error()}
		}
		// 无工具调用：智能体给出最终答复
		if len(msg.ToolCalls) == 0 {
			lastReply = messageContentString(msg.Content)
			if lastReply == "" {
				lastReply = "任务已执行完成"
			}
			break
		}

		// 纪律：每一轮只执行第一个工具调用（一个原子操作，一步只做一件事）
		tc := msg.ToolCalls[0]
		ac := msg.Content
		if ac == nil {
			ac = ""
		}
		messages = append(messages, chatMessage{Role: "assistant", Content: ac, ToolCalls: []toolCall{tc}})

		// 规范阶段只允许 set_plan，防止模型跳步直接执行
		if agentStage == stagePlanning && tc.Function.Name != "set_plan" {
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: "{\"success\":false,\"error\":\"当前处于规范（计划）阶段，请先调用 set_plan 建立执行计划后再开始执行。\"}"})
			continue
		}

		args := parseArgs(tc.Function.Arguments)
		// 「图算数」差异检测：操作前先抓取当前窗口缩略图作为基线
		var beforeThumb *image.RGBA
		if agentStage == stageExecuting && isOperationTool(tc.Function.Name) {
			beforeThumb, _ = captureWindowThumb()
		}
		result := executeTool(tc.Function.Name, args)
		// 操作后抓取新缩略图并与基线对比，把客观画面差异回填给模型（替代纯靠模型「自述生效」）
		if beforeThumb != nil {
			if afterThumb, err := captureWindowThumb(); err == nil {
				changed, ratio, detail := diffWindowThumb(beforeThumb, afterThumb)
				var diffText string
				if changed {
					diffText = fmt.Sprintf("\n【差异检测】操作后画面有变化：%s（变化占比约 %.1f%%）——这从画面侧佐证了操作可能已生效，请再结合截图确认具体效果（如输入框文字/新消息/标签切换）。", detail, ratio*100)
				} else {
					diffText = fmt.Sprintf("\n【差异检测】操作后画面几乎无变化（%.1f%%）——从画面侧看不出效果，请务必结合截图判断操作是否真正生效；若未生效请重试或换方法，不要宣称成功。", ratio*100)
				}
				result.Text += diffText
			}
		}
		rb, _ := json.Marshal(result)
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})

		// 操作工具：把首个待执行计划项标记为执行中（便于观察阶段判断推进到哪一步）
		if isOperationTool(tc.Function.Name) {
			markFirstPendingRunning()
		}

		// 结束：finish 给出最终总结（finish 内部会校验计划完成态；未全部完成时会返回失败，回到循环继续）
		if tc.Function.Name == "finish" {
			if result.Success {
				lastReply = result.Text
				markAllDone()
				break
			}
			// finish 被拒（仍有计划项未完成）：不结束当前循环，留在循环内继续让模型补齐
			lastReply = ""
			continue
		}
	}

	if lastReply == "" {
		lastReply = "已达到最大执行轮次，任务暂停"
	}

	// 记录本轮对话并裁剪历史（保留最近 AgentMaxRounds 轮）
	historyMu.Lock()
	agentHistory = append(agentHistory, agentRound{User: text, Assistant: lastReply})
	if len(agentHistory) > AgentMaxRounds {
		agentHistory = agentHistory[len(agentHistory)-AgentMaxRounds:]
	}
	historyMu.Unlock()

	return Result{Success: true, Text: lastReply}
}

// buildRoundContext 构造每轮的观察+决策上下文（最新截图 + 任务历史 + 决策指令）。
// 「观察」阶段的关键载体：指示模型依据最新截图核实上一步是否生效，再决定步进/重试。
func buildRoundContext(round int) []contentPart {
	var sb strings.Builder
	fmt.Fprintf(&sb, "【第 %d 轮 · 桌面上下文】当前前台窗口：%s\n", round, getForegroundTitle())
	if len(agentTaskHistory) > 0 {
		sb.WriteString("【任务历史】\n")
		sb.WriteString(formatTaskHistory())
		sb.WriteString("\n")
	}
	sb.WriteString("【决策指令】请只规划并执行**一个**操作（本轮只调用一个工具）：\n")
	sb.WriteString("- 若上一步操作刚执行完，先依据最新截图判断它是否真正生效（目标窗口是否前台、输入框是否出现文字、是否进入目标标签、是否出现弹窗）。\n")
	sb.WriteString("- 生效：关键验收步用 confirm_step(no, passed=true) 标记完成，然后推进到下一步操作。\n")
	sb.WriteString("- 未生效：用同一个操作重试（可多次），或 wait 后再试；未确认完成前绝不要跳到下一步。\n")
	sb.WriteString("- 若仍在规范阶段，请先调用 set_plan。全部完成或无法继续推进时调用 finish。")

	parts := []contentPart{{Type: "text", Text: sb.String()}}
	if faceLTPUseVision {
		if img, err := screenshotImagePart(); err == nil {
			parts[0].Text += "\n<image>"
			parts = append(parts, img)
		}
	}
	return parts
}

// trimOldScreenshots 把较早的截图消息替换为文字占位，上下文中只保留最新一张截图，
// 防止多轮循环累积大量截图导致上下文膨胀、代理超时/502。
func trimOldScreenshots(messages []chatMessage) {
	var imgIdx []int
	for i := range messages {
		parts, ok := messages[i].Content.([]contentPart)
		if !ok {
			continue
		}
		for _, p := range parts {
			if p.Type == "image_url" {
				imgIdx = append(imgIdx, i)
				break
			}
		}
	}
	if len(imgIdx) <= 1 {
		return
	}
	keep := imgIdx[len(imgIdx)-1]
	for _, i := range imgIdx {
		if i == keep {
			continue
		}
		parts := messages[i].Content.([]contentPart)
		for j := range parts {
			switch parts[j].Type {
			case "image_url":
				parts[j] = contentPart{Type: "text", Text: "（较早的截图已省略，以最新截图为准）"}
			case "text":
				parts[j].Text = strings.ReplaceAll(parts[j].Text, "<image>", "")
			}
		}
		messages[i].Content = parts
	}
}

// resetTaskState 重置任务历史与阶段（每条指令开始时调用）
func resetTaskState() {
	agentTaskHistory = nil
	agentStage = stagePlanning
}

// formatTaskHistory 生成任务历史文本（供每轮上下文展示给模型）
func formatTaskHistory() string {
	if len(agentTaskHistory) == 0 {
		return "（尚无计划）"
	}
	var lines []string
	for _, it := range agentTaskHistory {
		lines = append(lines, fmt.Sprintf("%d.[%s%s] %s%s", it.No, statusTag(it), triesTag(it), it.Action, detailTag(it.Detail)))
	}
	return strings.Join(lines, "\n")
}

// statusTag 状态中文标签
func statusTag(it agentPlanItem) string {
	switch it.Status {
	case "pending":
		return "待执行"
	case "running":
		return "执行中"
	case "done":
		return "完成"
	case "retry":
		return "重试"
	}
	return it.Status
}

// triesTag 重试次数后缀（如 ×2）
func triesTag(it agentPlanItem) string {
	if it.Tries > 0 {
		return "×" + fmt.Sprint(it.Tries)
	}
	return ""
}

// detailTag 计划项说明后缀
func detailTag(d string) string {
	if d == "" {
		return ""
	}
	return "（" + d + "）"
}

// messageContentString 从模型 message.content 提取纯文本（兼容 string 与多模态数组）
func messageContentString(c any) string {
	switch v := c.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var texts []string
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					texts = append(texts, t)
				}
			}
		}
		return strings.TrimSpace(strings.Join(texts, "\n"))
	}
	return ""
}

// parseArgs 解析工具参数的 JSON 字符串为 map
func parseArgs(s string) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// ==== 参数取值辅助 ====

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func argBool(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

// parseHoldMs 解析鼠标/键盘按住语义：short=1秒，long/hold=10秒，其余 0（瞬时）
func parseHoldMs(hold string) int {
	switch strings.ToLower(strings.TrimSpace(hold)) {
	case "short":
		return 1000
	case "long", "hold":
		return 10000
	}
	return 0
}

// operationTools 判定是否桌面原子操作工具（用于把首个待执行计划项标记为执行中）
var operationTools = map[string]bool{
	"list_windows": true, "activate_window": true, "close_window": true,
	"launch_program": true, "open_folder": true, "click": true, "mouse": true,
	"mouse_drag": true, "move_mouse": true, "scroll_wheel": true,
	"type_text": true, "type_and_send": true, "press_key": true,
	"wait": true, "capture_screenshot": true,
}

func isOperationTool(name string) bool {
	return operationTools[name]
}

// markFirstPendingRunning 把首个待执行计划项标记为执行中
func markFirstPendingRunning() {
	for i := range agentTaskHistory {
		if agentTaskHistory[i].Status == "pending" {
			agentTaskHistory[i].Status = "running"
			return
		}
	}
}

// markAllDone 任务结束时把执行中/待执行的计划项统一置为完成
func markAllDone() {
	for i := range agentTaskHistory {
		if agentTaskHistory[i].Status == "running" || agentTaskHistory[i].Status == "pending" {
			agentTaskHistory[i].Status = "done"
		}
	}
}

// ==== 工具执行器 ====

// executeTool 按工具名分发执行，返回结构化结果（回填给模型）
func executeTool(name string, args map[string]any) toolResult {
	switch name {
	// —— 流程控制工具：规范 / 步进 / 结束 ——
	case "set_plan":
		return execSetPlan(args)
	case "confirm_step":
		return execConfirmStep(args)
	case "finish":
		return execFinish(args)

	// —— 窗口定位与程序操作 ——
	case "list_windows":
		wins := listWindows()
		if len(wins) == 0 {
			return toolResult{Success: true, Text: "未发现可见窗口"}
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "当前前台窗口：%s\n共 %d 个窗口：\n", getForegroundTitle(), len(wins))
		for _, w := range wins {
			fmt.Fprintf(&sb, "- %s（进程:%s, 类:%s, PID:%d）\n", w.Title, w.Process, w.Class, w.PID)
		}
		return toolResult{Success: true, Text: sb.String()}

	case "activate_window":
		t := strings.TrimSpace(argStr(args, "title"))
		if err := activateWindow(t); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: "已激活窗口「" + t + "」"}

	case "close_window":
		t := strings.TrimSpace(argStr(args, "title"))
		s, err := closeWindow(t)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: s}

	case "launch_program":
		n := strings.TrimSpace(argStr(args, "name"))
		s, err := launchProgram(n)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: s}

	case "open_folder":
		p := strings.TrimSpace(argStr(args, "path"))
		s, err := openFolder(p)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: s}

	// —— 统一鼠标原语（坐标一律为焦点窗口内相对坐标，自动换算屏幕绝对坐标） ——
	case "click":
		wx, wy := argInt(args, "x"), argInt(args, "y")
		sx, sy, err := windowCoordsToScreen(wx, wy)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		if err := mouseClick(sx, sy, "left", argBool(args, "double")); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		if argBool(args, "double") {
			return toolResult{Success: true, Text: fmt.Sprintf("已在窗口(%d,%d)（屏幕(%d,%d)）左键双击", wx, wy, sx, sy)}
		}
		return toolResult{Success: true, Text: fmt.Sprintf("已在窗口(%d,%d)（屏幕(%d,%d)）左键单击", wx, wy, sx, sy)}

	case "mouse":
		wx, wy := argInt(args, "x"), argInt(args, "y")
		btn := argStr(args, "button")
		if btn == "" {
			btn = "left"
		}
		hold := argStr(args, "hold")
		holdMs := parseHoldMs(hold)
		sx, sy, err := windowCoordsToScreen(wx, wy)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		if err := mouseButton(sx, sy, btn, holdMs); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: fmt.Sprintf("已在窗口(%d,%d)（屏幕(%d,%d)）%s按键(hold=%dms)", wx, wy, sx, sy, btn, holdMs)}

	case "mouse_drag":
		steps := argInt(args, "steps")
		if steps == 0 {
			steps = 10
		}
		if err := windowCoordsDrag(argInt(args, "x1"), argInt(args, "y1"), argInt(args, "x2"), argInt(args, "y2"), steps); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: fmt.Sprintf("已在窗口内从(%d,%d)拖拽到(%d,%d)", argInt(args, "x1"), argInt(args, "y1"), argInt(args, "x2"), argInt(args, "y2"))}

	case "move_mouse":
		wx, wy := argInt(args, "x"), argInt(args, "y")
		sx, sy, err := windowCoordsToScreen(wx, wy)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		if err := mouseMove(sx, sy); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: fmt.Sprintf("已移动光标到窗口(%d,%d)（屏幕(%d,%d)）", wx, wy, sx, sy)}

	case "scroll_wheel":
		wx, wy := argInt(args, "x"), argInt(args, "y")
		d := strings.TrimSpace(argStr(args, "direction"))
		sx, sy, err := windowCoordsToScreen(wx, wy)
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		if err := scrollWheelAt(sx, sy, d, argInt(args, "amount")); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: fmt.Sprintf("已在窗口(%d,%d)滚动滚轮：%s", wx, wy, d)}

	// —— 键盘与输入 ——
	case "type_text":
		t := argStr(args, "text")
		if err := typeText(t); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: "已键入文本"}

	case "type_and_send":
		s, err := typeAndSend(argStr(args, "text"))
		if err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: s}

	case "press_key":
		k := strings.TrimSpace(argStr(args, "key"))
		if err := pressKey(k); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: "已按下「" + k + "」"}

	// —— 等待与观察 ——
	case "wait":
		ms := argInt(args, "ms")
		if ms <= 0 {
			ms = 1000
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return toolResult{Success: true, Text: fmt.Sprintf("已等待 %dms", ms)}

	case "capture_screenshot":
		if _, err := captureAnnotatedScreen(); err != nil {
			return toolResult{Success: false, Error: err.Error()}
		}
		return toolResult{Success: true, Text: "已刷新最新焦点窗口截图（叠加坐标网格），请依据它观察与定位"}

	default:
		return toolResult{Success: false, Error: "未知工具: " + name}
	}
}

// ==== 流程控制工具实现 ====

// execSetPlan 规范阶段：登记任务历史，进入执行阶段
func execSetPlan(args map[string]any) toolResult {
	rawPlan, ok := args["plan"].([]any)
	if !ok || len(rawPlan) == 0 {
		return toolResult{Success: false, Error: "set_plan 需要非空的 plan 数组"}
	}
	var plan []agentPlanItem
	for i, raw := range rawPlan {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		no := i + 1
		if n, ok := m["no"].(float64); ok {
			no = int(n)
		}
		action := ""
		if s, ok := m["action"].(string); ok {
			action = strings.TrimSpace(s)
		}
		if action == "" {
			continue
		}
		detail := ""
		if s, ok := m["detail"].(string); ok {
			detail = s
		}
		plan = append(plan, agentPlanItem{No: no, Action: action, Detail: detail, Status: "pending"})
	}
	if len(plan) == 0 {
		return toolResult{Success: false, Error: "set_plan 计划项为空，请给出具体的操作计划"}
	}
	agentTaskHistory = plan
	agentStage = stageExecuting
	return toolResult{Success: true, Text: fmt.Sprintf("已建立执行计划，共 %d 项，进入执行阶段。开始逐项执行（一次一个操作，逐步观察验证）。\n%s", len(plan), formatTaskHistory())}
}

// execConfirmStep 步进：标记计划项完成或需重试
func execConfirmStep(args map[string]any) toolResult {
	no := argInt(args, "no")
	if no == 0 {
		return toolResult{Success: false, Error: "confirm_step 需要计划项序号 no"}
	}
	passed := argBool(args, "passed")
	note := argStr(args, "note")
	for i := range agentTaskHistory {
		if agentTaskHistory[i].No == no {
			if passed {
				agentTaskHistory[i].Status = "done"
				if note != "" {
					return toolResult{Success: true, Text: fmt.Sprintf("计划项 %d 已确认完成。 说明：%s", no, note)}
				}
				return toolResult{Success: true, Text: fmt.Sprintf("计划项 %d 已确认完成。", no)}
			}
			agentTaskHistory[i].Status = "retry"
			agentTaskHistory[i].Tries++
			if note != "" {
				return toolResult{Success: true, Text: fmt.Sprintf("计划项 %d 标记为未命中，将重试。 说明：%s", no, note)}
			}
			return toolResult{Success: true, Text: fmt.Sprintf("计划项 %d 标记为未命中，将重试。", no)}
		}
	}
	return toolResult{Success: false, Error: fmt.Sprintf("未找到计划项 %d", no)}
}

// execFinish 结束：给出最终总结。
// 强制校验：若仍有计划项处于 pending/running/retry（未 done），拒绝提前结束，
// 防止模型「没做完就 finish」造成漏执行/半途而废。
func execFinish(args map[string]any) toolResult {
	var undone []string
	for _, it := range agentTaskHistory {
		if it.Status != "done" {
			undone = append(undone, fmt.Sprintf("#%d[%s]%s", it.No, statusTag(it), it.Action))
		}
	}
	if len(undone) > 0 {
		return toolResult{Success: false, Error: "还有计划项未完成，不能结束：" + strings.Join(undone, "；") + "。请继续逐个操作并确认完成（confirm_step passed=true），或用 wait/重试推进；确实无法完成时，明确如实说明失败原因后再 finish。"}
	}
	summary := strings.TrimSpace(argStr(args, "summary"))
	if summary == "" {
		summary = "任务已完成"
	}
	return toolResult{Success: true, Text: summary}
}
