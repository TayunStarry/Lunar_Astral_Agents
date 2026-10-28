package BilibiliViewer

// preselect.go 启动阶段：预选候选、采集视频可获取的文本信息、生成视频印象、综合排序。
//
// 采集范围（能取到就取）：标题、标签、简介、时长、发布时间、发布者、最多 30 条评论
// （单条超过 200 字直接硬截断）、最多 30 条实时弹幕。
//
// 排序：嵌入模型计算「视频印象」与本轮观影意图的相似度，再叠加播放/点赞等热度加权，
// 综合得分从高到低排序。这里只做排序，不做淘汰——淘汰交给主模型审核。

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// preselect 预选 4-8 个候选：首页优先，不够再按关键词搜索补足。
func (r *runner) preselect() []videoLink {
	want := candidateMin + randIntn(candidateMax-candidateMin+1)
	seen := seenSnapshot()
	pool := make([]videoLink, 0, want+8)

	home, err := r.browser.collectVideoLinks(60)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取首页视频卡片失败: %v", err)
	} else {
		pool = append(pool, prepareCandidates(home, r.cfg, seen)...)
	}

	keywords := append(append([]string{}, r.cfg.DesireKeywords...), r.cfg.PreferenceKeywords...)
	for _, keyword := range pickSearchKeywords(keywords, searchKeywordTries) {
		if len(dedupeLinks(pool)) >= want {
			break
		}
		searchURL := "https://search.bilibili.com/all?keyword=" + url.QueryEscape(keyword)
		logger.Info("BilibiliViewer", "候选不足，按关键词搜索补充: %s", keyword)
		if err := r.browser.navigate(searchURL, 25*time.Second); err != nil {
			continue
		}
		r.browser.wait(2 * time.Second)
		found, ferr := r.browser.collectVideoLinks(30)
		if ferr != nil {
			continue
		}
		pool = append(pool, prepareCandidates(found, r.cfg, seen)...)
	}

	pool = dedupeLinks(pool)
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Score > pool[j].Score })
	if len(pool) > want {
		pool = pool[:want]
	}
	logger.Info("BilibiliViewer", "预选候选 %d 个（目标 %d 个）", len(pool), want)
	return pool
}

// dedupeLinks 按视频 URL 去重，保留首次出现。
func dedupeLinks(links []videoLink) []videoLink {
	out := make([]videoLink, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		key := strings.TrimSpace(link.Href)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, link)
	}
	return out
}

// collectVideos 逐个采集候选视频的信息；采集失败的直接跳过。
func (r *runner) collectVideos(links []videoLink) {
	for index, link := range links {
		r.setProgress(fmt.Sprintf("正在采集候选 %d/%d：%s", index+1, len(links), truncate(link.Title, 30)))
		record := r.collectOne(link)
		if record == nil {
			continue
		}
		r.videos = append(r.videos, record)
	}
	logger.Info("BilibiliViewer", "候选采集完成：%d 个", len(r.videos))
}

// collectOne 采集单个视频的标题、标签、简介、时长、发布时间、发布者、评论与弹幕。
func (r *runner) collectOne(link videoLink) *videoRecord {
	bvid := bvidOf(link.Href)
	if bvid == "" {
		logger.Warn("BilibiliViewer", "候选链接里没有 BV 号，跳过: %s", link.Href)
		return nil
	}
	info, err := fetchVideoInfo(bvid)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取视频信息失败（跳过）: %s: %v", link.Href, err)
		return nil
	}
	if info.Title == "" {
		info.Title = link.Title
	}
	markSeen(bvid)

	tags, err := fetchVideoTags(bvid)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取视频标签失败（继续）: %s: %v", info.Title, err)
		tags = nil
	}

	if err := r.browser.navigate(link.Href, 30*time.Second); err != nil {
		logger.Warn("BilibiliViewer", "打开候选详情页失败（改用接口采集）: %v", err)
	}
	r.browser.wait(1200 * time.Millisecond)

	return &videoRecord{
		Link:     link,
		Info:     info,
		Tags:     tags,
		Comments: r.readCommentsDeep(info.Aid),
		Danmaku:  fetchDanmaku(info.Cid, collectDanmakuLimit),
	}
}

// readCommentsDeep 分页读取候选评论，单条超过 collectCommentMaxRunes 直接硬截断。
func (r *runner) readCommentsDeep(aid int64) []commentItem {
	if aid <= 0 {
		return nil
	}
	out := make([]commentItem, 0, collectCommentLimit)
	seen := make(map[string]bool, collectCommentLimit)
	for page := 1; page <= collectPageCount && len(out) < collectCommentLimit; page++ {
		items, err := r.browser.readCommentsAPIPage(1, aid, page, collectPageSize, false)
		if err != nil || len(items) == 0 {
			break
		}
		for _, item := range items {
			key := item.User + "\x00" + item.Content
			if seen[key] {
				continue
			}
			seen[key] = true
			item.Content = hardTruncate(item.Content, collectCommentMaxRunes)
			out = append(out, item)
			if len(out) >= collectCommentLimit {
				break
			}
		}
	}
	if len(out) == 0 {
		if dom, err := r.browser.readComments(collectCommentLimit); err == nil {
			for _, item := range dom {
				item.Content = hardTruncate(item.Content, collectCommentMaxRunes)
				out = append(out, item)
			}
		}
	}
	return out
}

// buildImpressions 用轻量模型为每个候选生成结构化「视频印象」。
//
// 这一步只用 light_reasoning；完成后调用方会立即卸载它，再加载主模型做审核。
func (r *runner) buildImpressions() {
	for index, video := range r.videos {
		if video == nil {
			continue
		}
		r.setProgress(fmt.Sprintf("正在整理视频印象 %d/%d：《%s》", index+1, len(r.videos), video.Info.Title))
		impression := buildOneImpression(video)
		video.Impression = impression
		if impression.Risky {
			video.Risky = true
			video.RiskReason = impression.RiskReason
		}
	}
	logger.Info("BilibiliViewer", "视频印象整理完成：%d 个", len(r.videos))
}

// buildOneImpression 单条视频印象：材料标准化排版后交给轻量模型归纳。
func buildOneImpression(video *videoRecord) videoImpression {
	material := impressionMaterial(video)
	task := "你是 B 站内容速读助手。根据下面采集到的视频信息（标题、标签、简介、时长、发布时间、" +
		"UP主、评论、弹幕），从侧面判断这个视频大概讲了什么、观众是什么情绪。" +
		"只输出一个 JSON 对象，不要解释、不要代码块：\n" +
		`{"summary":"一两句话概括视频内容","topics":["主题词1","主题词2"],"tone":"观众情绪","risky":false,"risk_reason":""}` + "\n" +
		"要求：summary ≤120 字；topics 2-5 个；不确定的地方用「看起来/大概是」表述，不要编造具体情节；" +
		"如果明显涉及政治敏感、色情低俗、暴力血腥、赌博诈骗、民族宗教争议、地域或性别对立，risky 置 true 并写原因。"
	text, err := chatTextModel(modelLight, task, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "《%s》印象生成失败，改用采集信息兜底: %v", video.Info.Title, err)
		return fallbackImpression(video)
	}
	var impression videoImpression
	if raw := extractJSONObject(text); raw != "" {
		if err := json.Unmarshal([]byte(raw), &impression); err != nil {
			logger.Warn("BilibiliViewer", "《%s》印象 JSON 解析失败，改用兜底: %v", video.Info.Title, err)
			return fallbackImpression(video)
		}
	}
	impression.Summary = strings.TrimSpace(impression.Summary)
	if impression.Summary == "" {
		return fallbackImpression(video)
	}
	for index, topic := range impression.Topics {
		impression.Topics[index] = strings.TrimSpace(topic)
	}
	impression.Summary = truncate(impression.Summary, 160)
	return impression
}

// fallbackImpression 模型不可用时的印象兜底：标题 + 标签 + 简介摘要。
func fallbackImpression(video *videoRecord) videoImpression {
	summary := fmt.Sprintf("《%s》，UP主 %s。%s", video.Info.Title, video.Info.Owner,
		hardTruncate(strings.TrimSpace(video.Info.Desc), 120))
	return videoImpression{
		Summary: strings.TrimSpace(summary),
		Topics:  append([]string{}, video.Tags...),
		Tone:    "未知",
	}
}

// impressionMaterial 标准化排版后的采集材料。
func impressionMaterial(video *videoRecord) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "《%s》\n", video.Info.Title)
	fmt.Fprintf(&builder, "UP主：%s\n", video.Info.Owner)
	fmt.Fprintf(&builder, "时长：%d 秒\n", video.Info.Duration)
	if video.Info.PublishTime > 0 {
		fmt.Fprintf(&builder, "发布时间：%s\n", time.Unix(video.Info.PublishTime, 0).Format("2006-01-02"))
	}
	if len(video.Tags) > 0 {
		fmt.Fprintf(&builder, "标签：%s\n", strings.Join(video.Tags, "、"))
	}
	if desc := strings.TrimSpace(video.Info.Desc); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", hardTruncate(desc, 600))
	}
	fmt.Fprintf(&builder, "播放：%d｜点赞：%d｜收藏：%d｜评论：%d｜弹幕：%d\n",
		video.Info.View, video.Info.Like, video.Info.Favorite, video.Info.ReplyNum, video.Info.Danmaku)
	if len(video.Comments) > 0 {
		builder.WriteString("评论：\n")
		for index, comment := range video.Comments {
			if index >= collectCommentLimit {
				break
			}
			fmt.Fprintf(&builder, "- %s：%s\n", comment.User, comment.Content)
		}
	}
	if len(video.Danmaku) > 0 {
		sample := video.Danmaku
		if len(sample) > collectDanmakuLimit {
			sample = sample[:collectDanmakuLimit]
		}
		fmt.Fprintf(&builder, "弹幕：%s\n", strings.Join(sample, " / "))
	}
	return strings.TrimSpace(builder.String())
}

// rankVideos 计算综合匹配度并降序排序。
//
// 综合分 = 0.7 × 嵌入相似度 + 0.3 × 热度分（播放/点赞/收藏取对数归一）。
func (r *runner) rankVideos() {
	scores := r.embeddingScores(r.intentText())
	for index, video := range r.videos {
		similarity := 0.0
		if scores != nil && index < len(scores) {
			similarity = scores[index]
		}
		video.Score = 0.7*similarity + 0.3*popularityScore(video.Info)
	}
	sort.SliceStable(r.videos, func(i, j int) bool { return r.videos[i].Score > r.videos[j].Score })
	for index, video := range r.videos {
		if index >= 8 {
			break
		}
		logger.Info("BilibiliViewer", "候选排序 %d：《%s》综合 %.3f（印象：%s）",
			index+1, video.Info.Title, video.Score, truncate(video.Impression.Summary, 40))
	}
}

// popularityScore 热度归一：播放量占主导，点赞/收藏各占一部分，结果落在 0-1。
func popularityScore(info videoInfo) float64 {
	view := math.Log10(float64(info.View) + 1)
	like := math.Log10(float64(info.Like) + 1)
	favorite := math.Log10(float64(info.Favorite) + 1)
	raw := 0.6*view + 0.25*like + 0.15*favorite
	// 播放 1000 万（log≈7）视为满分附近
	return clampFloat(raw/7.0, 0, 1)
}

// intentText 本轮观影意图：指令 + 关键词 + 长期记忆。
func (r *runner) intentText() string {
	var builder strings.Builder
	if r.instruction != "" {
		fmt.Fprintf(&builder, "本轮想法：%s\n", r.instruction)
	}
	if len(r.cfg.DesireKeywords) > 0 {
		fmt.Fprintf(&builder, "本轮关键词：%s\n", strings.Join(r.cfg.DesireKeywords, "、"))
	}
	if len(r.cfg.PreferenceKeywords) > 0 {
		fmt.Fprintf(&builder, "长期偏好：%s\n", strings.Join(r.cfg.PreferenceKeywords, "、"))
	}
	if memory := recentMemoryText(12); memory != "" {
		fmt.Fprintf(&builder, "我的观影记忆：\n%s\n", memory)
	}
	return strings.TrimSpace(builder.String())
}

// embeddingScores 计算每个候选印象与观影意图的余弦相似度；失败返回 nil。
func (r *runner) embeddingScores(intent string) []float64 {
	if len(r.videos) == 0 || strings.TrimSpace(intent) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), embeddingTimeout)
	defer cancel()

	documents := make([]string, len(r.videos))
	for index, video := range r.videos {
		documents[index] = impressionText(video)
	}
	vectors, err := memory.EmbedDocuments(ctx, documents)
	if err != nil || len(vectors) != len(documents) {
		logger.Warn("BilibiliViewer", "候选向量计算失败（退回热度排序）: %v", err)
		return nil
	}
	query, err := memory.EmbedQuery(ctx, intent)
	if err != nil || len(query) == 0 {
		logger.Warn("BilibiliViewer", "观影意图向量计算失败（退回热度排序）: %v", err)
		return nil
	}
	scores := make([]float64, len(vectors))
	for index, vector := range vectors {
		scores[index] = cosineSimilarity(query, vector)
	}
	return scores
}

// impressionText 把视频印象拼成用于向量化的短文本。
func impressionText(video *videoRecord) string {
	if video == nil {
		return ""
	}
	parts := []string{video.Info.Title, video.Impression.Summary}
	parts = append(parts, video.Impression.Topics...)
	parts = append(parts, video.Tags...)
	return strings.Join(parts, " ")
}

// cosineSimilarity 计算两个向量的余弦相似度（维度不一致时返回 -1）。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot, normA, normB float64
	for index := range a {
		dot += float64(a[index]) * float64(b[index])
		normA += float64(a[index]) * float64(a[index])
		normB += float64(b[index]) * float64(b[index])
	}
	if normA == 0 || normB == 0 {
		return -1
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
