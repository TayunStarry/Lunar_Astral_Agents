package thinking

// fast_ack.go 复合工具的快速应答。
//
// 触发粒度是「本轮工具调用是否请求了复合工具」，只看工具名，不解析分支参数：
//   - dispatch_technique（技法创作）
//   - network_explore（联网探索）
//   - use_the_program（扩展应用）
//
// 同一轮内多次调用同类（或不同类）复合工具只推一次，由 fastAckSent 去重；
// 台词候选集中在 variable.go 的 fastAckLines / fastAckOpenings / fastAckEndings。

// maybeSendFastAck 复合工具的快速应答（在工具执行前调用）
func maybeSendFastAck(functionName string) {
	stateMu.Lock()
	if fastAckSent {
		stateMu.Unlock()
		return
	}
	stateMu.Unlock()
	text := fastAckText(functionName)
	if text == "" {
		return
	}
	stateMu.Lock()
	fastAckSent = true
	stateMu.Unlock()
	// 快速应答恒定标记 auto=true：它是执行长任务前的旁白，永远不消费 QQ 的应答份额。
	if audio := synthesizeSpeech(text); audio != "" {
		fastAckPushHook("text", text, audio, true)
		return
	}
	fastAckPushHook("text", text, "", true)
}

// fastAckText 按工具名取正文候选并随机拼装；非复合工具返回空串
func fastAckText(functionName string) string {
	actions := fastAckLines[functionName]
	if len(actions) == 0 {
		return ""
	}
	return joinAckParts([]string{
		pickAckText(fastAckOpenings),
		pickAckText(actions),
		pickAckText(fastAckEndings),
	})
}
