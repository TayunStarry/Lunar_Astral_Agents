package MutualForum

// prompt.go 系统提示词：角色规则 + 当前连接与身份上下文。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/prompts"
)

// buildSystemPrompt 读取内嵌提示词并附加当前论坛状态。
func buildSystemPrompt(status *Status) (string, error) {
	base, err := prompts.Read("mutualForum.md")
	if err != nil {
		return "", fmt.Errorf("读取互助论坛提示词失败: %w", err)
	}
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(string(base)))

	if status == nil {
		builder.WriteString("\n\n【当前状态】\n- 不可用")
		return builder.String(), nil
	}
	builder.WriteString("\n\n【当前状态】")
	builder.WriteString("\n- 论坛地址：")
	builder.WriteString(status.BaseURL)
	if status.Reachable {
		builder.WriteString("\n- 连接：正常")
	} else {
		builder.WriteString("\n- 连接：")
		builder.WriteString(firstNonEmpty(status.Error, "不可用"))
	}
	if status.Health != nil {
		builder.WriteString(fmt.Sprintf("\n- 申领模式：%s；轮询间隔 %d 秒；轮次有效期 %d 秒",
			status.Health.EnrollMode, status.Health.PollIntervalSeconds, status.Health.RoundTTLSeconds))
	}
	if status.Identity != nil {
		builder.WriteString(fmt.Sprintf("\n- 当前身份：%s（%s）", status.Identity.Name, status.Identity.UID))
		if status.Identity.NameGenerated {
			builder.WriteString("（网名由你自己生成）")
		}
		if status.Me != nil {
			builder.WriteString(fmt.Sprintf("\n- 统计：发起话题 %d 个；现存发言 %d 条",
				status.Me.Stats.TopicsCreated, status.Me.Stats.PostsSent))
		}
	} else if status.IdentityError != "" {
		builder.WriteString("\n- 当前身份：")
		builder.WriteString(status.IdentityError)
	} else {
		builder.WriteString("\n- 当前身份：尚未申领（首次写操作会自动申领）")
	}
	if status.Identity != nil && status.Identity.RoundID != "" {
		builder.WriteString("\n- 当前轮次：")
		builder.WriteString(status.Identity.RoundID)
	} else {
		builder.WriteString("\n- 当前轮次：未领取（发言前工具会自动轮询）")
	}
	return builder.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
