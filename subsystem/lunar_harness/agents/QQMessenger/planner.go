package QQMessenger

// planner.go 投递计划：把自然语言需求解析为「目标 + 文本 + 句柄」的结构化计划。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/transport/napcat"
)

// planSend 调用规划模型，返回一次投递计划。
func planSend(instruction string, groups []napcat.QQGroup, friends []napcat.QQFriend) (sendPlan, error) {
	system, err := prompts.Read("qqMessenger.md")
	if err != nil {
		return sendPlan{}, fmt.Errorf("读取 专项通讯提示词失败: %w", err)
	}
	text, err := chatText(string(system), buildPlanMaterial(instruction, groups, friends))
	if err != nil {
		return sendPlan{}, err
	}
	var plan sendPlan
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &plan); err != nil {
		return sendPlan{}, fmt.Errorf("解析投递计划失败: %w（模型输出：%s）", err, truncate(text, 160))
	}
	plan.normalize()
	return plan, nil
}

// normalize 收敛模型输出：多目标清单逐项去空白，并兼容旧的单目标字段。
func (plan *sendPlan) normalize() {
	plan.Reason = strings.TrimSpace(plan.Reason)
	out := make([]sendDelivery, 0, len(plan.Deliveries)+1)
	for _, delivery := range plan.Deliveries {
		delivery.TargetType = strings.ToLower(strings.TrimSpace(delivery.TargetType))
		delivery.Target = strings.TrimSpace(delivery.Target)
		delivery.Text = strings.TrimSpace(delivery.Text)
		delivery.Reason = strings.TrimSpace(delivery.Reason)
		out = append(out, delivery)
	}
	if len(out) == 0 && strings.TrimSpace(plan.Target) != "" {
		out = append(out, sendDelivery{
			TargetType: strings.ToLower(strings.TrimSpace(plan.TargetType)),
			Target:     strings.TrimSpace(plan.Target),
			Text:       strings.TrimSpace(plan.Text),
			Images:     plan.Images,
			Files:      plan.Files,
			Reason:     plan.Reason,
		})
	}
	plan.Deliveries = out
}

// buildPlanMaterial 组装模型输入：可投递目标清单 + 对话者需求。
func buildPlanMaterial(instruction string, groups []napcat.QQGroup, friends []napcat.QQFriend) string {
	var builder strings.Builder
	builder.WriteString("【可投递目标】\n")
	if len(groups) == 0 && len(friends) == 0 {
		builder.WriteString("（无）\n")
	}
	for index, group := range groups {
		if index >= maxDirectoryEntries {
			fmt.Fprintf(&builder, "（其余 %d 个群省略）\n", len(groups)-maxDirectoryEntries)
			break
		}
		fmt.Fprintf(&builder, "群 %d「%s」\n", group.GroupID, strings.TrimSpace(group.GroupName))
	}
	for index, friend := range friends {
		if index >= maxDirectoryEntries {
			fmt.Fprintf(&builder, "（其余 %d 个好友省略）\n", len(friends)-maxDirectoryEntries)
			break
		}
		name := strings.TrimSpace(friend.Nickname)
		if remark := strings.TrimSpace(friend.Remark); remark != "" {
			name = remark + "（" + name + "）"
		}
		fmt.Fprintf(&builder, "好友 %d「%s」\n", friend.UserID, name)
	}
	builder.WriteString("\n【对话者需求】\n")
	builder.WriteString(strings.TrimSpace(instruction))
	return builder.String()
}

// extractJSONObject 从模型输出中截出第一个 JSON 对象（容忍代码块包裹）。
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return strings.TrimSpace(text)
	}
	return text[start : end+1]
}
