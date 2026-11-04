package ViewerCensor

// agent.go 观影者-审查：只做合规与风险判定（附一句观影评价），不生成对外文案。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/agents/viewbase"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Review 用主模型复核合规性，并生成视频评价与切片数；返回模型原始文本，由链解析。
//
// 首次带封面做多模态复核；封面缺失或调用失败后降级纯文本重试。
func Review(ctx context.Context, in ReviewInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptReview)
	if system == "" {
		return "", fmt.Errorf("读取审查提示词失败")
	}
	system = renderPrompt(system,
		"{defaultSlices}", itoa(in.DefaultSlices),
		"{minSlices}", itoa(in.MinSlices),
		"{maxSlices}", itoa(in.MaxSlices))
	var lastErr error
	for attempt := 0; attempt < reviewAttempts; attempt++ {
		var images []string
		if in.Cover != "" && attempt == 0 {
			images = []string{in.Cover}
		}
		text, lastErr = viewbase.Chat(ctx, viewbase.ChatRequest{
			Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material,
			Images: images, Attempts: 1,
		})
		if lastErr == nil {
			recordReview(in.Title, text)
			return text, nil
		}
		if attempt+1 < reviewAttempts {
			backoff := time.Duration(attempt+1) * retryBackoff
			logger.Warn("BilibiliViewer", "《%s》审核第 %d 次失败，%v 后重试: %v", in.Title, attempt+1, backoff, lastErr)
			time.Sleep(backoff)
		}
	}
	return "", lastErr
}

// itoa 整数转字符串（提示词占位符替换用）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		digits = append(digits, byte('0'+v%10))
		v /= 10
	}
	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
