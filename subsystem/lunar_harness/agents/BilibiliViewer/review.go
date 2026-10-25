package BilibiliViewer

// review.go 选片审核：一次模型调用同时给出「是否通过 / 未通过原因 / 需要查看的切片数」。
//
// 审核与「看几个片段」共用同一份标题/封面/简介：分成两轮只会多一次等待，
// 还容易出现「审核说安全、切片判定又问一遍」的重复。输出统一为结构化 JSON，
// 失败时按高风险处理（宁可错杀），切片数再按时长收敛。

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// videoReview 选片审核的结构化结果：是否通过、未通过原因、本轮要看的切片数。
type videoReview struct {
	Pass   bool
	Reason string
	Slices int
}

// reviewVideo 选片审核：内置风险词 + 多模态复核，一次调用同时给出审核结论与切片数。
// 模型不可用或输出无法解析时按高风险处理（宁可错杀）。
func reviewVideo(info videoInfo) videoReview {
	if verdict, hit := ruleRisk(info.Title + "\n" + info.Desc); hit {
		return videoReview{Reason: verdict.Reason, Slices: defaultSlicesPerVideo}
	}
	review, err := aiReviewVideo(info)
	if err != nil {
		logger.Warn("BilibiliViewer", "视频审核模型不可用或输出无法解析，按高风险处理（宁可错杀）: %v", err)
		return videoReview{Reason: "审核模型不可用或输出无法解析", Slices: defaultSlicesPerVideo}
	}
	if review.Pass {
		review.Reason = ""
		review.Slices = clampReviewSlices(info.Duration, review.Slices)
	} else {
		review.Reason = strings.TrimSpace(review.Reason)
		if review.Reason == "" {
			review.Reason = "模型审核未通过"
		}
		review.Slices = defaultSlicesPerVideo
	}
	reason := review.Reason
	if reason == "" {
		reason = "（无）"
	}
	logger.Info("BilibiliViewer", "视频审核结果：《%s》通过=%v｜原因=%s｜切片=%d",
		info.Title, review.Pass, reason, review.Slices)
	return review
}

// aiReviewVideo 用多模态模型复核风险并决定切片数；要求输出结构化 JSON。
func aiReviewVideo(info videoInfo) (videoReview, error) {
	maxSlices := sliceLimitForDuration(info.Duration)
	minSlices := sliceMinCount
	if minSlices > maxSlices {
		minSlices = maxSlices
	}
	defaultWant := clampReviewSlices(info.Duration, defaultSlicesPerVideo)
	system := fmt.Sprintf("你是 B 站内容安全审核员兼观影编排器。判断视频是否适合让一个女孩子公开留言，并决定本轮看几个 60 秒片段。"+
		"只输出一个 JSON 对象，不要解释、不要代码块：\n"+
		`{"pass": true, "reason": "", "slices": %d}`+"\n"+
		"规则：pass=false 时 reason 写具体原因（政治敏感、社会对立、违法违规、色情低俗、赌博诈骗、暴力血腥、"+
		"民族宗教争议、地域或性别对立、煽动情绪、人肉网暴、引流诈骗等）；拿不准也判 false。"+
		"slices 按视频实际时长在 %d-%d 之间选择（短视频少看几个、长视频可以多看几个），拿不准时取 %d。",
		defaultWant, minSlices, maxSlices, defaultWant)
	material := riskMaterial(info, nil) +
		fmt.Sprintf("\n时长：%d 秒\n最多可看切片：%d 个（每片 60 秒）", info.Duration, maxSlices)
	cover := fetchCoverDataURL(info.Pic)
	var lastErr error
	for attempt := 0; attempt < reviewAttempts; attempt++ {
		var text string
		var err error
		if cover != "" {
			text, err = chatWithImagesOnce(system, material, []string{cover})
		} else {
			text, err = chatCompletionOnce([]chatMessage{
				{Role: "system", Content: system},
				{Role: "user", Content: material},
			}, nil, loopguard.Options{})
		}
		if err == nil {
			var review videoReview
			if review, err = parseVideoReview(text, maxSlices); err == nil {
				return review, nil
			}
		}
		lastErr = err
		if attempt+1 < reviewAttempts {
			logger.Warn("BilibiliViewer", "视频审核第 %d 次失败，重试一次: %v", attempt+1, err)
		}
	}
	return videoReview{}, lastErr
}

// parseVideoReview 解析审核 JSON；兼容模型退回旧式 RISK/SAFE 文本的情况。
func parseVideoReview(text string, maxSlices int) (videoReview, error) {
	if raw := extractJSONObject(text); raw != "" {
		var payload struct {
			Pass   *bool  `json:"pass"`
			Reason string `json:"reason"`
			Slices *int   `json:"slices"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.Pass != nil {
			review := videoReview{
				Pass:   *payload.Pass,
				Reason: strings.TrimSpace(payload.Reason),
				Slices: defaultSlicesPerVideo,
			}
			if payload.Slices != nil && *payload.Slices > 0 {
				review.Slices = *payload.Slices
			}
			if review.Slices > maxSlices {
				review.Slices = maxSlices
			}
			if review.Slices > sliceMaxCount {
				review.Slices = sliceMaxCount
			}
			return review, nil
		}
	}
	// 兼容旧式输出：明确 SAFE/RISK 时仍按文本判定，切片数取第一个整数
	upper := strings.ToUpper(text)
	if strings.Contains(upper, "SAFE") || strings.Contains(text, "无风险") || strings.Contains(text, "没有风险") {
		slices := firstInt(text)
		if slices <= 0 {
			slices = defaultSlicesPerVideo
		}
		if slices > maxSlices {
			slices = maxSlices
		}
		return videoReview{Pass: true, Slices: slices}, nil
	}
	if strings.Contains(upper, "RISK") || strings.Contains(text, "风险") {
		return videoReview{Reason: "模型复核：" + truncate(strings.TrimSpace(text), 60), Slices: defaultSlicesPerVideo}, nil
	}
	return videoReview{}, fmt.Errorf("审核结果无法解析: %s", truncate(strings.TrimSpace(text), 80))
}

// extractJSONObject 从模型输出里截出第一个 JSON 对象（容忍 ```json 代码块等包裹）
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// sliceLimitForDuration 按视频时长算出最多能看几个 60 秒片段（1 到 sliceMaxCount）
func sliceLimitForDuration(duration int) int {
	if duration <= 0 {
		return sliceMaxCount
	}
	available := (duration + sliceSec - 1) / sliceSec
	return clampInt(available, 1, sliceMaxCount)
}

// clampReviewSlices 把模型给的切片数收敛到「2-8 且不超过视频可用片段数」
func clampReviewSlices(duration, want int) int {
	limit := sliceLimitForDuration(duration)
	if want <= 0 {
		want = defaultSlicesPerVideo
	}
	if limit < sliceMinCount {
		return limit
	}
	return clampInt(want, sliceMinCount, limit)
}

// firstInt 取文本里的第一个整数（模型偶尔会带解释或标点）
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
