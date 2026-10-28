package DialogueBridging

import (
	"strings"
	"testing"

)

func testDirectory() ([]QQGroup, []QQFriend) {
	groups := []QQGroup{
		{GroupID: 1001, GroupName: "测试群"},
		{GroupID: 1002, GroupName: "测试群二"},
	}
	friends := []QQFriend{
		{UserID: 2001, Nickname: "小明"},
		{UserID: 2002, Nickname: "小红", Remark: "同桌"},
	}
	return groups, friends
}

// TestResolveTargetByIDAndName 钉住目标解析：号码优先、名称唯一匹配，歧义时报错。
func TestResolveTargetByIDAndName(t *testing.T) {
	groups, friends := testDirectory()
	cases := []struct {
		name string
		plan sendPlan
		want int64
		kind string
	}{
		{"群号", sendPlan{TargetType: "group", Target: "1001"}, 1001, "group"},
		{"群名", sendPlan{TargetType: "group", Target: "测试群"}, 1001, "group"},
		{"好友号", sendPlan{TargetType: "user", Target: "2002"}, 2002, "user"},
		{"好友备注", sendPlan{TargetType: "user", Target: "同桌"}, 2002, "user"},
		{"无类型按号码推断", sendPlan{Target: "2001"}, 2001, "user"},
		{"无类型按名称推断", sendPlan{Target: "测试群"}, 1001, "group"},
	}
	for _, c := range cases {
		target, err := resolveTarget(c.plan, groups, friends)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", c.name, err)
		}
		if target.ID != c.want || target.Kind != c.kind {
			t.Fatalf("%s 应为 %s/%d，实际 %s/%d", c.name, c.kind, c.want, target.Kind, target.ID)
		}
	}

	if _, err := resolveTarget(sendPlan{TargetType: "group", Target: "测试"}, groups, friends); err == nil {
		t.Fatal("名称匹配到多个群时应报错")
	}
	if _, err := resolveTarget(sendPlan{TargetType: "group", Target: "9999"}, groups, friends); err == nil {
		t.Fatal("号码不在目录中时应报错")
	}
}

// TestHandleMentioned 需求里的句柄校验：大小写不敏感，忽略锚点。
func TestHandleMentioned(t *testing.T) {
	if !handleMentioned("@D.003|L1-5", "把 @D.003 发出去") {
		t.Fatal("带锚点的句柄应能匹配到基础句柄")
	}
	if !handleMentioned("@i.001", "这张 @I.001 图发群里") {
		t.Fatal("句柄比对应忽略大小写")
	}
	if handleMentioned("@D.009", "把 @D.003 发出去") {
		t.Fatal("需求中没出现的句柄不应通过")
	}
}

// TestDeliveryReport 钉住给对话者的简要报告：数量与失败原因都要在。
func TestDeliveryReport(t *testing.T) {
	outcome := deliveryOutcome{TextSent: true, ImagesSent: 2, Failures: []string{"@D.009：需求中没有提到这个句柄"}}
	report := outcome.report(resolvedTarget{Kind: "group", ID: 1001, Name: "测试群"})
	for _, want := range []string{"测试群", "1001", "文本 1 条", "图片 2 张", "@D.009"} {
		if !strings.Contains(report, want) {
			t.Fatalf("报告缺少 %q：%s", want, report)
		}
	}
	if strings.Contains(report, "文件") {
		t.Fatalf("没有文件时不应出现文件数量：%s", report)
	}
}
