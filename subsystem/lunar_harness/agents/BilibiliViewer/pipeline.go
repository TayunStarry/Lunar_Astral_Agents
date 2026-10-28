package BilibiliViewer

// pipeline.go 启动阶段：一次性完成选片前的全部准备工作，然后交给 Tick 观看。
//
//	1) 打开浏览器并确认登录
//	2) 处理被动的 @ 与追评（有就回，没有就跳过）
//	3) 结合指令、记忆与配置生成本轮搜索关键词
//	4) 立即卸载主推理模型，释放显存
//	5) 预选 4-8 个候选并采集视频信息（标题/标签/简介/时长/发布时间/UP主/评论/弹幕）
//	6) 用轻量模型生成结构化「视频印象」
//	7) 嵌入匹配 + 热度加权，综合匹配度降序排序
//	8) 卸载轻量模型，切回主模型做合规审核、视频评价与切片数
//	9) 生成 Tick 计划表：先发布动态与评论区，再逐个切片观看 → 合并入库

import (
	"math/rand"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// startup 执行启动阶段；成功时浏览器保持打开，等待 Tick 继续观看。
func (r *runner) startup() (text string, err error) {
	defer func() {
		if err != nil {
			r.cleanup()
			r.finished = true
			dispatchBeforeLifeEnds(r, "failed", err.Error())
		}
	}()

	if err = r.openBrowser(); err != nil {
		return r.resultText(), err
	}
	r.mid = r.browser.ownMid()

	// 清掉历史遗留的机械化记忆与视频摘要（切片过程文本 / 运行统计）
	if removed := cleanupMechanicalMemories(); removed > 0 {
		logger.Info("BilibiliViewer", "已清理 %d 条机械化观影记忆", removed)
	}
	if removed := cleanupMechanicalVideoSummaries(); removed > 0 {
		logger.Info("BilibiliViewer", "已清理 %d 条机械化视频摘要", removed)
	}

	// 第一步：本轮专属搜索关键词（启动时首先确定本轮要看什么）
	r.setProgress("正在结合指令、记忆与配置生成本轮搜索关键词…")
	r.prepareDesireKeywords()

	// 被动消息：只有收到 @ 或追评时才回应，不主动去视频评论区留言
	r.handleNotifications()

	// 立即卸载主推理模型，给后面的轻量印象整理腾显存
	unloadModel(modelPrimary)

	// 预选与采集
	candidates := r.preselect()
	if len(candidates) == 0 {
		r.setProgress("本轮没有预选到候选视频，结束")
		r.chargeQuota()
		r.finished = true
		r.cleanup()
		recordRoundMemory(r)
		dispatchBeforeLifeEnds(r, "finished", "")
		return r.resultText(), nil
	}
	r.collectVideos(candidates)

	// 视频印象（轻量模型）
	if len(r.videos) > 0 {
		r.buildImpressions()
	}

	// 综合匹配度排序
	r.rankVideos()

	// 切换回主模型做审核与评价：先卸掉轻量模型，主模型按需加载
	unloadModel(modelLight)

	r.approved = make([]*videoRecord, 0, len(r.videos))
	for index, video := range r.videos {
		if video == nil {
			continue
		}
		r.setProgress("正在审核候选 " + itoa(index+1) + "/" + itoa(len(r.videos)) + "：《" + video.Info.Title + "》")
		review := reviewVideo(video)
		if !review.Pass {
			video.Risky = true
			video.RiskReason = review.Reason
			logger.Info("BilibiliViewer", "《%s》审核未通过：%s", video.Info.Title, review.Reason)
			continue
		}
		video.Slices = review.Slices
		video.Evaluation = review.Evaluation
		if video.Evaluation == "" {
			video.Evaluation = truncate(video.Impression.Summary, 60)
		}
		r.approved = append(r.approved, video)
	}

	if len(r.approved) == 0 {
		r.setProgress("候选视频都没有通过审核，结束")
		r.chargeQuota()
		r.finished = true
		r.cleanup()
		recordRoundMemory(r)
		dispatchBeforeLifeEnds(r, "finished", "")
		return r.resultText(), nil
	}

	r.buildSteps()
	logger.Info("BilibiliViewer", "启动阶段完成：通过 %d 个视频，共 %d 个 Tick 步骤",
		len(r.approved), len(r.steps))
	return r.resultText(), nil
}

// buildSteps 生成 Tick 计划表。
//
// 动态与评论区只需要「视频印象 + 视频评价」，不依赖观看切片，
// 所以第一步先发动态与评论，之后每个视频再 N 个切片步骤 + 1 个合并步骤。
func (r *runner) buildSteps() {
	steps := make([]pipelineStep, 0, len(r.approved)*(watchSliceMax+1)+1)
	steps = append(steps, pipelineStep{Kind: stepPublish})
	for _, video := range r.approved {
		if video == nil {
			continue
		}
		slices := clampReviewSlices(video.Info.Duration, video.Slices)
		video.Slices = slices
		for index := 0; index < slices; index++ {
			steps = append(steps, pipelineStep{Kind: stepWatch, Video: video, Slice: index})
		}
		steps = append(steps, pipelineStep{Kind: stepMerge, Video: video})
	}
	r.steps = steps
}

// openBrowser 打开浏览器并确保登录。
func (r *runner) openBrowser() error {
	r.setProgress("正在打开 B 站浏览器…")
	browser, err := openBilibiliBrowser()
	if err != nil {
		return err
	}
	r.browser = browser
	if err := browser.ensureLogin(); err != nil {
		return err
	}
	return nil
}

// randIntn 返回 [0,n) 的随机整数（n<=0 时返回 0）。
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// hardTruncate 硬截断到指定字数，不追加省略号（采集评论要求）。
func hardTruncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

// itoa 简单整数转字符串（避免额外引入 strconv 到多个文件）。
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
