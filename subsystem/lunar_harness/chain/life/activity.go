package life

// activity.go 「我刚刚在干的事情」状态文案的唯一收敛点。
//
// 生活事件的完成、失败、条件不满足早退都从这里落到同一条状态字段上：
// 分支自己写过更具体文案时由 ActivityRun 声明（优先），否则兜底用步骤记录；
// 失败与早退分开措辞，避免用户看到过期状态或分不清「没做成」与「做不了」。

import (
	"errors"
	"fmt"
	"strings"
)

// setActivity 写入状态字段；空文案不覆盖当前状态。
func setActivity(text string) {
	if setActivityHook == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	setActivityHook(truncateAtSentence(text, activityTextLimit))
}

// activityTextOf 取本步骤应写入的状态文案：分支文案优先，其次步骤记录。
func activityTextOf(run EventRun, step EventStep) string {
	if specific, ok := run.(ActivityRun); ok {
		if text := strings.TrimSpace(specific.ActivityText()); text != "" {
			return text
		}
	}
	return strings.TrimSpace(step.Record)
}

// skip 构造一个「条件不满足」的早退原因。
func skip(reason string) error {
	return &skipError{reason: reason}
}

// isSkip 判断错误是否属于条件不满足的早退。
func isSkip(err error) bool {
	var target *skipError
	return errors.As(err, &target)
}

// failureActivity 失败 / 早退时的状态文案：两者措辞不同，便于区分。
func failureActivity(eventID string, err error) string {
	reason := "没有说明"
	if err != nil {
		reason = truncateAtSentence(err.Error(), activityReasonLimit)
	}
	if isSkip(err) {
		return fmt.Sprintf("想%s，但这会儿做不了（%s）", eventAction(eventID), reason)
	}
	return fmt.Sprintf("想%s，但没做成（%s）", eventAction(eventID), reason)
}

// eventAction 事件在「想做但没做成」类文案里的动词短语。
func eventAction(eventID string) string {
	switch eventID {
	case "watch":
		return "看点视频"
	case "forum":
		return "去互助论坛逛逛"
	case "creative":
		return "写点东西"
	case "daze":
		return "发会儿呆"
	case roundStageWatchlist:
		return "把待看列表补一补"
	case roundStageBiliReply:
		return "去 B 站回消息"
	case roundStageForumReply:
		return "去互助论坛回消息"
	default:
		return "找点事做"
	}
}
