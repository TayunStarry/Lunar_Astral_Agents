package BilibiliViewer

// engage.go Tick 观看阶段：先发布动态与评论区，再逐切片观看、合并入库。
//
// 每个 Tick 只做一个原子步骤：
//   - stepPublish 发布观影动态，在动态评论区输出视频评价与 URL，再发一条 @分享评论（只需印象与评价）；
//   - stepWatch   下载（首次）并理解一个 60 秒切片；
//   - stepMerge   合并该视频全部切片理解并写入记忆库。

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

// runNextStep 执行计划表里的下一个步骤；失败时下一个 Tick 重试一次，仍失败则跳过。
func (r *runner) runNextStep() {
	if r == nil || r.stepIndex >= len(r.steps) {
		return
	}
	step := r.steps[r.stepIndex]
	var err error
	switch step.Kind {
	case stepWatch:
		err = r.runWatchStep(step)
	case stepMerge:
		err = r.runMergeStep(step)
	case stepPublish:
		err = r.runPublishStep()
	}
	if err != nil {
		r.stepRetry++
		if r.stepRetry <= sliceRetryLimit {
			logger.Warn("BilibiliViewer", "步骤失败，下一个 Tick 重试（%d/%d）: %v",
				r.stepRetry, sliceRetryLimit, err)
			return
		}
		logger.Warn("BilibiliViewer", "步骤重试仍失败，跳过: %v", err)
	}
	r.stepRetry = 0
	r.stepIndex++
}

// runWatchStep 理解一个切片；第一次进入某视频时先下载并按期望段数切片。
func (r *runner) runWatchStep(step pipelineStep) error {
	video := step.Video
	if video == nil {
		return fmt.Errorf("切片步骤缺少视频")
	}
	if err := r.prepareSegments(video); err != nil {
		return err
	}
	if step.Slice >= len(video.Segments) {
		return fmt.Errorf("切片下标越界（%d/%d）", step.Slice, len(video.Segments))
	}
	if len(video.Summaries) < len(video.Segments) {
		video.Summaries = make([]string, len(video.Segments))
	}
	if strings.TrimSpace(video.Summaries[step.Slice]) != "" {
		return nil // 已理解过，直接进入下一片
	}

	segment := video.Segments[step.Slice]
	r.setProgress(fmt.Sprintf("正在看《%s》第 %d/%d 段…",
		video.Info.Title, step.Slice+1, len(video.Segments)))
	text, err := understandOneSlice(video.LocalPath, video.Info, segment, step.Slice+1, len(video.Segments))
	if err != nil {
		if errors.Is(err, loopguard.ErrLoop) {
			return fmt.Errorf("第 %d 段触发重复输出", step.Slice+1)
		}
		return fmt.Errorf("第 %d 段理解失败: %w", step.Slice+1, err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("第 %d 段理解为空", step.Slice+1)
	}
	video.Summaries[step.Slice] = text
	// 按要求把该片段的理解内容输出到运行日志（Tick 成果）
	logger.Info("BilibiliViewer", "《%s》第 %d/%d 段理解：%s",
		video.Info.Title, step.Slice+1, len(video.Segments), truncate(text, 300))
	return nil
}

// prepareSegments 首次观看某视频时下载并按期望段数切分。
func (r *runner) prepareSegments(video *videoRecord) error {
	if len(video.Segments) > 0 {
		return nil
	}
	slices := clampReviewSlices(video.Info.Duration, video.Slices)
	video.Slices = slices

	playURL, err := fetchPlayURL(video.Info.Bvid, video.Info.Cid, 32)
	if err != nil {
		return fmt.Errorf("获取播放直链失败: %w", err)
	}
	seconds := int(float64(slices) * module.MediaChunkSeconds)
	if video.Info.Duration > 0 && video.Info.Duration < seconds {
		seconds = video.Info.Duration
	}
	local, err := downloadClip(playURL, video.Info.Bvid, seconds)
	if err != nil {
		return fmt.Errorf("下载视频失败: %w", err)
	}
	segments, err := prepareVideoSegments(local, slices)
	if err != nil {
		_ = os.Remove(local)
		return fmt.Errorf("切分视频失败: %w", err)
	}
	video.LocalPath = local
	video.Segments = segments
	return nil
}

// runMergeStep 合并单视频全部切片理解，写入记忆库，并释放临时文件。
func (r *runner) runMergeStep(step pipelineStep) error {
	video := step.Video
	if video == nil {
		return fmt.Errorf("合并步骤缺少视频")
	}
	summaries := make([]string, 0, len(video.Summaries))
	for _, text := range video.Summaries {
		if strings.TrimSpace(text) != "" {
			summaries = append(summaries, text)
		}
	}
	if len(summaries) == 0 {
		return fmt.Errorf("《%s》没有可合并的切片理解", video.Info.Title)
	}
	// 切片理解只作素材：先合成人格化观影感受，再按 URL 写入摘要库与记忆库。
	video.WatchSummary = summarizeWatch(video, summaries)
	if video.WatchSummary != "" {
		storeVideoSummary(video.Info, video.WatchSummary)
		recordWatchMemory(video)
	} else {
		logger.Warn("BilibiliViewer", "《%s》观影感受生成失败，跳过摘要库与记忆写入（机械切片文本不落库）",
			video.Info.Title)
	}

	video.Segments = nil
	if video.LocalPath != "" {
		_ = os.Remove(video.LocalPath)
		video.LocalPath = ""
	}
	reply := video.WatchSummary
	if strings.TrimSpace(reply) == "" {
		reply = video.Impression.Summary
	}
	r.logActivity(lifeActivity{
		Kind:  "watch",
		Where: "《" + video.Info.Title + "》",
		Reply: truncate(reply, 200),
	})
	logger.Info("BilibiliViewer", "《%s》观影感受已入库：%s",
		video.Info.Title, truncate(reply, 200))
	return nil
}

// runPublishStep 发布观影动态，等评论区可用后逐条输出视频评价，最后发一条 @分享评论。
func (r *runner) runPublishStep() error {
	r.setProgress("正在写这一轮的观影动态…")
	text := generateDynamicText(r)
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("观影动态为空")
	}
	dynID, err := r.browser.publishDiary(text)
	if err != nil {
		return fmt.Errorf("发布观影动态失败: %w", err)
	}
	r.dynID = dynID
	r.dynamicText = text
	logger.Info("BilibiliViewer", "观影动态已发布（%s）", dynID)
	r.logActivity(lifeActivity{Kind: "dynamic", Where: "我的 B 站动态", Reply: truncate(text, 200)})

	// 动态 ID 不等于评论 oid：先读详情拿到真正的 comment_type / comment_id_str。
	target, err := r.browser.resolveDynamicCommentTarget(dynID)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取动态评论区目标失败，按纯文字动态回退: %v", err)
		target = dynamicCommentTarget{Type: 17, Oid: dynID}
	}
	logger.Info("BilibiliViewer", "动态评论区目标：type=%d oid=%s", target.Type, target.Oid)

	// 评论区可能仍在审核/索引，先确认可读写，再开始评论
	if err := r.browser.waitDynamicCommentAreaReady(target.Type, target.Oid); err != nil {
		logger.Warn("BilibiliViewer", "动态评论区未确认可用，仍尝试评论: %v", err)
	}

	// 动态评论区：一个视频一条「URL + 评价」，逐条发送并回读校验
	posted := 0
	for _, video := range r.approved {
		comment := buildVideoComment(video)
		if comment == "" {
			continue
		}
		if posted > 0 {
			r.browser.wait(dynamicCommentGap)
		}
		if err := r.browser.postDynamicCommentVerified(target.Type, target.Oid, comment, nil); err != nil {
			logger.Warn("BilibiliViewer", "动态评论区输出《%s》评价失败: %v", video.Info.Title, err)
			continue
		}
		video.CommentText = comment
		posted++
		logger.Info("BilibiliViewer", "动态评论区已输出第 %d 条视频评价：《%s》", posted, video.Info.Title)
	}
	if posted == 0 {
		logger.Warn("BilibiliViewer", "动态评论区未成功输出任何视频评价")
	}

	// 额外一条 @分享评论：放在评价之后，避免与新动态、链接评论同时触发审核
	if share := r.generateShareComment(); share != "" {
		targets := r.pickMentionTargets()
		body := share
		if len(targets) > 0 {
			names := make([]string, 0, len(targets))
			for _, target := range targets {
				names = append(names, target.Name)
			}
			body = strings.TrimSpace(share + " @" + strings.Join(names, " @"))
		}
		r.browser.wait(dynamicCommentGap)
		if err := r.browser.postDynamicCommentVerified(target.Type, target.Oid, body, targets); err != nil {
			logger.Warn("BilibiliViewer", "发 @分享评论失败: %v", err)
		} else {
			logger.Info("BilibiliViewer", "@分享评论已发布（@%d 人）：%s", len(targets), body)
			r.logActivity(lifeActivity{
				Kind:  "share",
				Where: "我的 B 站动态评论区",
				Reply: body,
				Note:  mentionNote(targets),
			})
		}
	}

	// 本轮记忆等全部切片看完后由 finishRound 统一写入（这里只发布，不结算）
	return nil
}

// buildVideoComment 动态评论区的一条条目：视频 URL 一行、评价一行。
func buildVideoComment(video *videoRecord) string {
	if video == nil || strings.TrimSpace(video.Info.URL) == "" {
		return ""
	}
	evaluation := strings.TrimSpace(video.Evaluation)
	if evaluation == "" {
		evaluation = truncate(video.Impression.Summary, 60)
	}
	evaluation = finishComment(evaluation, dynamicCommentMaxRunes)
	return strings.TrimSpace(video.Info.URL + "\n" + evaluation)
}

// generateShareComment 生成一条 @分享评论（基于本轮整体感受，不逐条枚举）。
func (r *runner) generateShareComment() string {
	if len(r.approved) == 0 {
		return ""
	}
	video := r.approved[rand.Intn(len(r.approved))]
	summary := strings.TrimSpace(video.WatchSummary)
	if summary == "" {
		summary = video.Impression.Summary
	}
	task := "写一句想把这一轮挑中的视频分享给朋友的话，像跟熟人随口安利，一句话、30 字内；" +
		"不复述视频内容、不提记忆里其他事。只输出正文；不要 @、不要链接。"
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的初步印象：%s\n",
		video.Info.Title, video.Info.Owner, truncate(summary, 300))
	text, err := chatTextModel(modelPrimary, task, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "生成分享评论失败: %v", err)
		return ""
	}
	return finishComment(strings.ReplaceAll(text, "@", ""), 50)
}

// pickMentionTargets 从特别关注列表随机挑 1-3 人。
func (r *runner) pickMentionTargets() []MentionTarget {
	followings := r.browser.specialFollowings(r.mid)
	if len(followings) == 0 {
		logger.Info("BilibiliViewer", "特别关注列表为空，本条分享不 @ 人")
		return nil
	}
	limit := mentionMaxCount
	if limit > len(followings) {
		limit = len(followings)
	}
	want := mentionMinCount + randIntn(limit-mentionMinCount+1)
	rand.Shuffle(len(followings), func(i, j int) {
		followings[i], followings[j] = followings[j], followings[i]
	})
	return followings[:want]
}

// specialFollowings 读取当前账号特别关注列表（special=1 或 tag 含 -10）。
func (w *webSession) specialFollowings(mid int64) []MentionTarget {
	if mid <= 0 {
		return nil
	}
	out := make([]MentionTarget, 0, 8)
	seen := make(map[int64]bool, 8)
	for page := 1; page <= 5; page++ {
		target := fmt.Sprintf(
			"https://api.bilibili.com/x/relation/followings?vmid=%d&pn=%d&ps=50",
			mid, page)
		raw, err := w.pageFetchText("GET", target, "", "", 20*time.Second)
		if err != nil {
			logger.Warn("BilibiliViewer", "读取特别关注列表失败: %v", err)
			break
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				List []struct {
					Mid     int64   `json:"mid"`
					Uname   string  `json:"uname"`
					Special int     `json:"special"`
					Tag     []int64 `json:"tag"`
				} `json:"list"`
			} `json:"data"`
		}
		if err := jsonUnmarshal(raw, &resp); err != nil || resp.Code != 0 {
			logger.Warn("BilibiliViewer", "解析特别关注列表失败: %v / code=%d", err, resp.Code)
			break
		}
		for _, user := range resp.Data.List {
			if user.Mid <= 0 || seen[user.Mid] {
				continue
			}
			if user.Special != 1 && !containsInt64(user.Tag, -10) {
				continue
			}
			seen[user.Mid] = true
			out = append(out, MentionTarget{Name: strings.TrimSpace(user.Uname), UID: user.Mid})
		}
		if len(resp.Data.List) < 50 {
			break
		}
	}
	logger.Info("BilibiliViewer", "特别关注列表共 %d 人", len(out))
	return out
}

// mentionNote 记录本轮 @ 了谁（写记忆用）。
func mentionNote(targets []MentionTarget) string {
	if len(targets) == 0 {
		return "未 @ 人"
	}
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return "@" + strings.Join(names, "、")
}

func containsInt64(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// finishRound 一轮结束：写本轮记忆（此时观看已全部完成）、清理浏览器、通知插件。
func (r *runner) finishRound() {
	recordRoundMemory(r)
	r.finished = true
	r.cleanup()
	r.setProgress("本轮观影完成")
	dispatchBeforeLifeEnds(r, "finished", "")
}
