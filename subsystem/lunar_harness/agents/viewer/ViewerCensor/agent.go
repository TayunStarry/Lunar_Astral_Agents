package ViewerCensor

// agent.go 观影者-审查：只做合规与风险判定，不生成对外文案。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase，
// 结论必须由 submit_review 工具调用提交，正文不解析。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/viewer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Review 用主模型复核合规性，结论只从 submit_review 的参数读取。
//
// 首次带封面做多模态复核；封面缺失或调用/提交失败后降级纯文本重试。
func Review(ctx context.Context, in ReviewInput) (review Verdict, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptReview)
	if system == "" {
		return Verdict{}, fmt.Errorf("读取审查提示词失败")
	}
	var lastErr error
	for attempt := 0; attempt < reviewAttempts; attempt++ {
		var images []string
		if in.Cover != "" && attempt == 0 {
			images = []string{in.Cover}
		}
		call, callErr := viewer.Submit(ctx, viewer.SubmitRequest{
			Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material,
			Images: images, Tool: reviewTool, Attempts: 1,
		})
		if callErr == nil {
			review, callErr = parseReviewArgs(call.Function.Arguments)
			if callErr == nil {
				agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolApplied, "审查结论已受理")
				recordReview(in.Title, review)
				return review, nil
			}
			agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolNotApplied, callErr.Error())
		}
		lastErr = callErr
		if attempt+1 < reviewAttempts {
			backoff := time.Duration(attempt+1) * retryBackoff
			logger.Warn("BilibiliViewer", "《%s》审核第 %d 次失败，%v 后重试: %v", in.Title, attempt+1, backoff, lastErr)
			time.Sleep(backoff)
		}
	}
	return Verdict{}, lastErr
}

// parseReviewArgs 校验 submit_review 的参数：pass 必填，pass=false 时 reason 必须非空。
func parseReviewArgs(raw string) (Verdict, error) {
	var payload struct {
		Pass   *bool  `json:"pass"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Verdict{}, fmt.Errorf("审查参数无法解析: %w", err)
	}
	if payload.Pass == nil {
		return Verdict{}, fmt.Errorf("审查参数缺少 pass")
	}
	reason := strings.TrimSpace(payload.Reason)
	if !*payload.Pass && reason == "" {
		return Verdict{}, fmt.Errorf("审查参数缺少未通过原因")
	}
	if *payload.Pass {
		reason = ""
	}
	return Verdict{Pass: *payload.Pass, Reason: reason}, nil
}
