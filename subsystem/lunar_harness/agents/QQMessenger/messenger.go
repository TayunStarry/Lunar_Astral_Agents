package QQMessenger

// messenger.go QQ 投递智能体的对外入口：接收自然语言需求，规划并执行一次投递。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/transport/napcat"
)

// Ready 主动投递是否可用：QQ 桥接器已连接且能解析群/好友信息。
func Ready() bool {
	return napcat.BridgeReady()
}

// Run 接收对话者的自然语言需求，投递到指定群/好友并返回简要报告。
//
// 这里不把底层发送工具暴露给对话者：需求先由规划模型解析为结构化计划，
// 再由本包的私有工具校验句柄、解析目标并执行，保证投递可控、结果可回报。
func Run(instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("请用自然语言说明要发给谁、发什么")
	}
	if !Ready() {
		return "QQ 投递未就绪：桥接器未连接，或暂时读不到群/好友信息。", nil
	}

	runMu.Lock()
	defer runMu.Unlock()

	groups, friends, dirErr := napcat.QQTargets()
	if len(groups)+len(friends) == 0 && dirErr != nil {
		return "", fmt.Errorf("读取 QQ 投递目标失败: %w", dirErr)
	}
	plan, err := planSend(instruction, groups, friends)
	if err != nil {
		return "", err
	}
	logger.Info("QQMessenger", "投递计划：目标类型=%s 目标=%s 文本=%d 字 图片=%d 文件=%d 原因=%s",
		plan.TargetType, plan.Target, len([]rune(plan.Text)), len(plan.Images), len(plan.Files), plan.Reason)

	report, err := executePlan(plan, instruction, groups, friends)
	if err != nil {
		logger.Warn("QQMessenger", "投递失败：%s（%v）", report, err)
		return report, err
	}
	logger.Info("QQMessenger", "投递完成：%s", report)
	return report, nil
}
