package viewer

// review.go 主模型审核与评价：一次调用同时给出「是否通过 / 未通过原因 / 视频评价 / 切片数」。
//
// 审核材料是已整理好的视频印象与采集摘要，不再重新读评论区；
// 评价用于发布到自己的动态评论区，必须像真人看完后的主观短评，不能复述视频内容。

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ViewerCensor"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

// reviewVideo 主模型审核：内置风险词先挡一层，再让主模型复核并生成评价与切片数。
// 模型不可用或输出无法解析时按高风险处理（宁可错杀）。
func reviewVideo(video *videoRecord) videoReview {
	if verdict, hit := ruleRisk(video.Info.Title + "\n" + video.Info.Desc); hit {
		return videoReview{Reason: verdict.Reason, Slices: watchSliceDefault}
	}
	if verdict, hit := ruleRiskComments(video.Comments); hit {
		return videoReview{Reason: verdict.Reason, Slices: watchSliceDefault}
	}
	review, err := aiReviewVideo(video)
	if err != nil {
		logger.Warn("BilibiliViewer", "《%s》审核模型不可用或输出无法解析，按高风险处理: %v", video.Info.Title, err)
		return videoReview{Reason: "审核模型不可用或输出无法解析", Slices: watchSliceDefault}
	}
	if review.Pass {
		review.Reason = ""
		review.Slices = clampReviewSlices(video.Info.Duration, review.Slices)
		review.Evaluation = finishComment(review.Evaluation, dynamicCommentMaxRunes)
	} else {
		review.Reason = strings.TrimSpace(review.Reason)
		if review.Reason == "" {
			review.Reason = "模型审核未通过"
		}
		review.Slices = watchSliceDefault
	}
	reason := review.Reason
	if reason == "" {
		reason = "（无）"
	}
	logger.Info("BilibiliViewer", "审核结果：《%s》通过=%v｜原因=%s｜切片=%d",
		video.Info.Title, review.Pass, reason, review.Slices)
	return review
}

// aiReviewVideo 委托观影者-审查复核合规性，并生成视频评价与切片数；要求结构化 JSON。
func aiReviewVideo(video *videoRecord) (videoReview, error) {
	maxSlices := sliceLimitForDuration(video.Info.Duration)
	minSlices := watchSliceMin
	if minSlices > maxSlices {
		minSlices = maxSlices
	}
	defaultWant := clampReviewSlices(video.Info.Duration, watchSliceDefault)
	material := riskMaterial(video.Info, video.Comments) +
		"\n\n【视频印象】\n" + truncate(video.Impression.Summary, 512) +
		fmt.Sprintf("\n\n时长：%d 秒\n最多可看切片：%d 个（每片 60 秒）", video.Info.Duration, maxSlices)
	cover := fetchCoverDataURL(video.Info.Pic)
	text, err := ViewerCensor.Review(context.Background(), ViewerCensor.ReviewInput{
		Title:         video.Info.Title,
		Material:      material,
		Cover:         cover,
		Model:         modelPrimary,
		DefaultSlices: defaultWant,
		MinSlices:     minSlices,
		MaxSlices:     maxSlices,
	})
	if err != nil {
		return videoReview{}, err
	}
	return parseVideoReview(text, maxSlices)
}

// parseVideoReview 解析审核 JSON；兼容模型退回旧式 RISK/SAFE 文本的情况。
func parseVideoReview(text string, maxSlices int) (videoReview, error) {
	if raw := extractJSONObject(text); raw != "" {
		var payload struct {
			Pass       *bool  `json:"pass"`
			Reason     string `json:"reason"`
			Evaluation string `json:"evaluation"`
			Slices     *int   `json:"slices"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.Pass != nil {
			review := videoReview{
				Pass:       *payload.Pass,
				Reason:     strings.TrimSpace(payload.Reason),
				Evaluation: strings.TrimSpace(payload.Evaluation),
				Slices:     watchSliceDefault,
			}
			if payload.Slices != nil && *payload.Slices > 0 {
				review.Slices = *payload.Slices
			}
			if review.Slices > maxSlices {
				review.Slices = maxSlices
			}
			if review.Slices > watchSliceMax {
				review.Slices = watchSliceMax
			}
			return review, nil
		}
	}
	// 兼容旧式输出：明确 SAFE/RISK 时仍按文本判定，切片数取第一个整数
	upper := strings.ToUpper(text)
	if strings.Contains(upper, "SAFE") || strings.Contains(text, "无风险") || strings.Contains(text, "没有风险") {
		slices := firstInt(text)
		if slices <= 0 {
			slices = watchSliceDefault
		}
		if slices > maxSlices {
			slices = maxSlices
		}
		return videoReview{Pass: true, Slices: slices}, nil
	}
	if strings.Contains(upper, "RISK") || strings.Contains(text, "风险") {
		return videoReview{Reason: "模型复核：" + truncate(strings.TrimSpace(text), 512), Slices: watchSliceDefault}, nil
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

// sliceLimitForDuration 按视频时长算出最多能看几个 60 秒片段（1 到 watchSliceMax）
func sliceLimitForDuration(duration int) int {
	if duration <= 0 {
		return watchSliceMax
	}
	available := int((float64(duration) + module.MediaChunkSeconds - 1) / module.MediaChunkSeconds)
	return clampInt(available, 1, watchSliceMax)
}

// clampReviewSlices 把模型给的切片数收敛到「2-8 且不超过视频可用片段数」
func clampReviewSlices(duration, want int) int {
	limit := sliceLimitForDuration(duration)
	if want <= 0 {
		want = watchSliceDefault
	}
	if limit < watchSliceMin {
		return limit
	}
	return clampInt(want, watchSliceMin, limit)
}

// firstInt 取文本里的第一个整数
func firstInt(text string) int {
	start := -1
	for i, r := range text {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			n, _ := strconv.Atoi(text[start:i])
			return n
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(text[start:])
		return n
	}
	return 0
}
