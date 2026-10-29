package DialogueBridging

import (
	"strings"
	"testing"
)

// TestWhitelistGateBlocksAndRequiresConfirmation 钉住主动投递的白名单闸门：
// 白名单目标直接放行；非白名单目标固定拦截；只有同一目标 + 明确确认才放行一次。
func TestWhitelistGateBlocksAndRequiresConfirmation(t *testing.T) {
	resetOutboundTest(t)
	bridgeConfig.BridgingUsers = []int64{1001}

	allowedGroup := resolvedTarget{Kind: "group", ID: 1001, Name: "测试群"}
	if ok, message := authorizeActiveSend(allowedGroup, "发一条晚安"); !ok || message != "" {
		t.Fatalf("白名单目标应直接放行: ok=%v message=%q", ok, message)
	}

	blockedUser := resolvedTarget{Kind: "user", ID: 2001, Name: "小明"}
	ok, message := authorizeActiveSend(blockedUser, "给小明发消息")
	if ok {
		t.Fatal("非白名单目标不得放行")
	}
	if !strings.HasPrefix(message, whitelistBlockedText) {
		t.Fatalf("提示必须包含固定文本，实际 %q", message)
	}
	if !strings.Contains(message, "确认绕过白名单") {
		t.Fatalf("提示必须要求用户确认是否绕过白名单，实际 %q", message)
	}

	// 未明确确认：保持拦截
	if ok, _ := authorizeActiveSend(blockedUser, "再发一次"); ok {
		t.Fatal("未获得明确确认前不得发送")
	}

	// 同一目标 + 明确确认：允许发送一次
	if ok, _ := authorizeActiveSend(blockedUser, "确认绕过白名单，给小明发送晚安"); !ok {
		t.Fatal("明确确认后应允许同一目标绕过一次")
	}
	// 确认已消费：下一次同样文本仍应拦截
	if ok, _ := authorizeActiveSend(blockedUser, "确认绕过白名单，再发一次"); ok {
		t.Fatal("绕过确认只能消费一次")
	}

	// 确认不得沿用到其它目标
	otherUser := resolvedTarget{Kind: "user", ID: 3001, Name: "小红"}
	if ok, _ := authorizeActiveSend(otherUser, "确认绕过白名单，给小红发送"); ok {
		t.Fatal("确认不得沿用到其它目标")
	}
}

// TestExecutePlanWhitelistBlockedIsNotFailure 白名单拦截返回固定提示，不视为投递故障
func TestExecutePlanWhitelistBlockedIsNotFailure(t *testing.T) {
	resetOutboundTest(t)
	bridgeConfig.BridgingUsers = nil

	plan := sendPlan{Deliveries: []sendDelivery{{
		TargetType: "group",
		Target:     "1001",
		Text:       "晚安",
	}}}
	groups := []QQGroup{{GroupID: 1001, GroupName: "测试群"}}
	text, err := executePlan(plan, "给测试群发晚安", groups, nil)
	if err != nil {
		t.Fatalf("白名单拦截不应报投递失败: %v", err)
	}
	if !strings.HasPrefix(text, whitelistBlockedText) {
		t.Fatalf("应原样返回固定提示，实际 %q", text)
	}
}

// TestWhitelistGateUsesExactFixedText 固定提示文本必须与需求一致
func TestWhitelistGateUsesExactFixedText(t *testing.T) {
	resetOutboundTest(t)
	bridgeConfig.BridgingUsers = nil // 空白名单 = 默认全部拦截

	target := resolvedTarget{Kind: "group", ID: 1001, Name: "测试群"}
	_, message := authorizeActiveSend(target, "发一条消息")
	if !strings.HasPrefix(message, "对方不在白名单中, 请勿随意打扰他人") {
		t.Fatalf("固定提示文本不符: %q", message)
	}
}
