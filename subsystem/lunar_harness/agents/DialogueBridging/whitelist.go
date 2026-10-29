package DialogueBridging

// whitelist.go 主动投递的白名单闸门。
//
// 默认启用白名单模式：任何主动发送（文本 / 图片 / 文件）在执行前都必须先校验目标
// 是否在 bridging_users 白名单内。目标不在白名单时一律不发送，返回固定提示文本，
// 并登记一次「待绕过确认」；只有对话者在下一次需求中明确给出确认口令、且目标仍为
// 同一对象时，才允许本次绕过白名单发送。确认只对当次生效，用后即焚。

import (
	"strconv"
	"strings"
	"time"
)

// whitelistKey 目标的稳定标识（群与好友号码可能相同，必须带类型前缀）
func whitelistKey(target resolvedTarget) string {
	return target.Kind + ":" + strconv.FormatInt(target.ID, 10)
}

// targetWhitelisted 目标是否在允许主动投递的白名单内
func targetWhitelisted(target resolvedTarget) bool {
	return isAllowedTarget(target.ID)
}

// authorizeActiveSend 主动发送前的白名单闸门。
//
// 返回 allowed=true 表示可以继续发送；否则 message 为必须原样回给对话者的提示。
func authorizeActiveSend(target resolvedTarget, instruction string) (bool, string) {
	if targetWhitelisted(target) {
		return true, ""
	}

	key := whitelistKey(target)
	now := time.Now()

	whitelistPendingMu.Lock()
	defer whitelistPendingMu.Unlock()

	// 清理过期登记，避免 map 长期膨胀
	for pendingKey, deadline := range whitelistPending {
		if now.After(deadline) {
			delete(whitelistPending, pendingKey)
		}
	}

	if deadline, ok := whitelistPending[key]; ok && now.Before(deadline) {
		if hasBypassConfirmation(instruction) {
			// 确认只消费一次：发送后立即失效，避免后续需求继续绕过
			delete(whitelistPending, key)
			return true, ""
		}
		// 已提示过但还没给出明确确认：保持拦截，不重复登记
		return false, whitelistBlockedText + whitelistConfirmHint
	}

	// 首次拦截：登记待确认目标，强制提示对话者确认是否绕过
	whitelistPending[key] = now.Add(whitelistBypassTTL)
	return false, whitelistBlockedText + whitelistConfirmHint
}

// hasBypassConfirmation 需求文本中是否包含明确的绕过白名单口令
func hasBypassConfirmation(instruction string) bool {
	text := strings.TrimSpace(instruction)
	if text == "" {
		return false
	}
	for _, phrase := range whitelistConfirmPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// resetWhitelistPending 清空待确认登记（测试隔离用）
func resetWhitelistPending() {
	whitelistPendingMu.Lock()
	whitelistPending = map[string]time.Time{}
	whitelistPendingMu.Unlock()
}
