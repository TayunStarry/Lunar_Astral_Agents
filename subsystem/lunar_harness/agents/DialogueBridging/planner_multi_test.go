package DialogueBridging

// planner_multi_test.go 钉住多目标投递计划的归一化规则。

import "testing"

func TestSendPlanNormalizesMultipleDeliveries(t *testing.T) {
	plan := sendPlan{
		Deliveries: []sendDelivery{
			{TargetType: " GROUP ", Target: " 测试群 ", Text: " 晚安 ", Images: []string{"@I.001"}},
			{TargetType: "USER", Target: " 2001 ", Text: " 明早见 "},
		},
		Reason: " ok ",
	}
	plan.normalize()
	if len(plan.Deliveries) != 2 {
		t.Fatalf("应保留两个目标，实际 %d", len(plan.Deliveries))
	}
	if plan.Deliveries[0].TargetType != "group" || plan.Deliveries[0].Target != "测试群" || plan.Deliveries[0].Text != "晚安" {
		t.Fatalf("第一个目标归一化不符合预期: %#v", plan.Deliveries[0])
	}
	if plan.Deliveries[1].TargetType != "user" || plan.Deliveries[1].Target != "2001" || plan.Deliveries[1].Text != "明早见" {
		t.Fatalf("第二个目标归一化不符合预期: %#v", plan.Deliveries[1])
	}
	if plan.Reason != "ok" {
		t.Fatalf("整体原因应去空白，实际 %q", plan.Reason)
	}
}

func TestSendPlanFallsBackToLegacySingleTarget(t *testing.T) {
	plan := sendPlan{TargetType: "group", Target: "测试群", Text: "晚安"}
	plan.normalize()
	if len(plan.Deliveries) != 1 || plan.Deliveries[0].Target != "测试群" || plan.Deliveries[0].Text != "晚安" {
		t.Fatalf("旧单目标格式应兼容为一个投递项: %#v", plan.Deliveries)
	}
}
