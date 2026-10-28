package DialogueBridging

// messenger.go 通讯桥接智能体的对外入口：接收自然语言需求，规划并执行一次投递。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Ready 主动投递是否可用：QQ 桥接器已连接且能解析群/好友信息。
func Ready() bool {
	return BridgeReady()
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
		return "通讯桥接未就绪：桥接器未连接，或暂时读不到群/好友信息。", nil
	}

	runMu.Lock()
	defer runMu.Unlock()

	groups, friends, dirErr := QQTargets()
	if len(groups)+len(friends) == 0 && dirErr != nil {
		return "", fmt.Errorf("读取 通讯桥接目标失败: %w", dirErr)
	}
	plan, err := planSend(instruction, groups, friends)
	if err != nil {
		return "", err
	}
	logger.Info("DialogueBridging", "投递计划：目标数=%d 原因=%s", len(plan.Deliveries), plan.Reason)
	for index, delivery := range plan.Deliveries {
		logger.Info("DialogueBridging", "  [%d] %s/%s 文本=%d 字 图片=%d 文件=%d",
			index+1, delivery.TargetType, delivery.Target,
			len([]rune(delivery.Text)), len(delivery.Images), len(delivery.Files))
	}

	report, err := executePlan(plan, instruction, groups, friends)
	recordDelivery(instruction, plan, report, err == nil)
	if err != nil {
		logger.Warn("DialogueBridging", "投递失败：%s（%v）", report, err)
		return report, err
	}
	logger.Info("DialogueBridging", "投递完成：%s", report)
	return report, nil
}

// recordDelivery 保留最近一次投递，供「数据导出」读取通讯桥接上下文。
func recordDelivery(instruction string, plan sendPlan, report string, success bool) {
	lastMu.Lock()
	defer lastMu.Unlock()
	lastDelivery = &deliveryRecord{
		Instruction: instruction,
		Deliveries:  append([]sendDelivery(nil), plan.Deliveries...),
		Report:      report,
		Success:     success,
		At:          time.Now(),
	}
}
