package thinking

// fast_ack.go 复合工具的快速应答。
//
// 触发粒度是「本轮工具调用是否请求了复合工具」，只看工具名，不解析分支参数：
//   - dispatch_technique（技法创作）
//   - network_explore（联网探索）
//   - use_the_program（扩展应用）
//
// 同一轮内多次调用同类（或不同类）复合工具只推一次，由 fastAckSent 去重；
// 台词候选集中在 variable.go 的 fastAckLines / fastAckOpenings / fastAckEndings，
// 三段的舍弃概率是同文件的 fastAckOpeningDrop / fastAckActionDrop / fastAckEndingDrop。

// maybeSendFastAck 复合工具的快速应答（在工具执行前调用）
func maybeSendFastAck(functionName string) {
	// 判定与置位必须在同一临界区：否则并发调用（多工具同轮执行）会各推一次旁白
	stateMu.Lock()
	if fastAckSent {
		stateMu.Unlock()
		return
	}
	text := fastAckText(functionName)
	if text == "" {
		stateMu.Unlock()
		return
	}
	fastAckSent = true
	stateMu.Unlock()
	// 快速应答恒定标记 auto=true：它是执行长任务前的旁白，永远不消费 QQ 的应答份额。
	if audio := synthesizeSpeech(text); audio != "" {
		fastAckPushHook("text", text, audio, true)
		return
	}
	fastAckPushHook("text", text, "", true)
}

// fastAckText 按工具名取三段候选并拼装；非复合工具返回空串。
//
// 顺序固定为「开场 → 动作 → 收尾」，三段各自独立抽签决定是否缺席；
// 若三段同时缺席，用正文兜底，保证不会推出空消息。
func fastAckText(functionName string) string {
	actions := fastAckLines[functionName]
	if len(actions) == 0 {
		return ""
	}
	text := joinAckParts([]string{
		pickAckPart(fastAckOpenings, fastAckOpeningDrop),
		pickAckPart(actions, fastAckActionDrop),
		pickAckPart(fastAckEndings, fastAckEndingDrop),
	})
	if text == "" {
		text = pickAckText(actions)
	}
	return text
}

// pickAckPart 按概率舍弃一段；未舍弃时从候选池随机取一条。
func pickAckPart(items []string, dropChance float64) string {
	if randomFloat(0, 1) < dropChance {
		return ""
	}
	return pickAckText(items)
}
