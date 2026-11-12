package viewer

// review.go 主模型审核：只回答「是否适合让一个女孩子公开讨论」。
//
// 审核材料由采集到的标题、简介、评论与标签组成，不再依赖采集期视频印象，
// 也不再把切片数交给模型决定——观看阶段固定只抽一个 1 分钟片段。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewer/ViewerCensor"
	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/wishchain"
)

// reviewVideo 主模型审核：内置风险词先挡一层，再让主模型复核。
// 模型不可用或输出无法解析时按高风险处理（宁可错杀）。
func reviewVideo(video *videoRecord) videoReview {
	if verdict, hit := ruleRisk(video.Info.Title + "\n" + video.Info.Desc); hit {
		return videoReview{Reason: verdict.Reason}
	}
	if verdict, hit := ruleRiskComments(video.Comments); hit {
		return videoReview{Reason: verdict.Reason}
	}
	review, err := aiReviewVideo(video)
	if err != nil {
		logger.Warn("BilibiliViewer", "《%s》审核模型不可用或输出无法解析，按高风险处理: %v", video.Info.Title, err)
		return videoReview{Reason: "审核模型不可用或输出无法解析"}
	}
	if review.Pass {
		review.Reason = ""
	} else {
		review.Reason = strings.TrimSpace(review.Reason)
		if review.Reason == "" {
			review.Reason = "模型审核未通过"
		}
	}
	reason := review.Reason
	if reason == "" {
		reason = "（无）"
	}
	logger.Info("BilibiliViewer", "审核结果：《%s》通过=%v｜原因=%s", video.Info.Title, review.Pass, reason)
	// 观影反馈接入心愿链路：过审带来满足，未通过带来遗憾。
	if review.Pass {
		wishchain.NoteLink(wishchain.LinkViewerFeedback, "《"+video.Info.Title+"》过审")
	} else {
		wishchain.NoteLink(wishchain.LinkViewerRejected, "《"+video.Info.Title+"》未通过："+reason)
	}
	return review
}

// aiReviewVideo 委托观影者-审查复核合规性；要求结构化 JSON。
func aiReviewVideo(video *videoRecord) (videoReview, error) {
	material := riskMaterial(video.Info, video.Comments)
	if len(video.Tags) > 0 {
		material += "\n\n【视频标签】" + strings.Join(video.Tags, "、")
	}
	if video.Info.Duration > 0 {
		material += fmt.Sprintf("\n\n【视频时长】%s", formatDuration(video.Info.Duration))
	}
	cover := bilibili.FetchCoverDataURL(video.Info.Pic)
	text, err := ViewerCensor.Review(context.Background(), ViewerCensor.ReviewInput{
		Title:    video.Info.Title,
		Material: material,
		Cover:    cover,
		Model:    modelPrimary,
	})
	if err != nil {
		return videoReview{}, err
	}
	return parseVideoReview(text)
}

// parseVideoReview 解析审核 JSON；兼容模型退回旧式 RISK/SAFE 文本的情况。
func parseVideoReview(text string) (videoReview, error) {
	if raw := extractJSONObject(text); raw != "" {
		var payload struct {
			Pass   *bool  `json:"pass"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.Pass != nil {
			return videoReview{
				Pass:   *payload.Pass,
				Reason: strings.TrimSpace(payload.Reason),
			}, nil
		}
	}
	// 兼容旧式输出：明确 SAFE/RISK 时仍按文本判定。
	upper := strings.ToUpper(text)
	if strings.Contains(upper, "SAFE") || strings.Contains(text, "无风险") || strings.Contains(text, "没有风险") {
		return videoReview{Pass: true}, nil
	}
	if strings.Contains(upper, "RISK") || strings.Contains(text, "风险") {
		return videoReview{Reason: "模型复核：" + truncate(strings.TrimSpace(text), 512)}, nil
	}
	return videoReview{}, fmt.Errorf("审核结果无法解析: %s", truncate(strings.TrimSpace(text), 512))
}

// extractJSONObject 从模型输出里截出第一个 JSON 对象（容忍代码块包裹）
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}
