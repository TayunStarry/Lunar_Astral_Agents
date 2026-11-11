package viewer

// quick_digest.go 指定视频的 URL 直驱印象。
//
// 只聚合公开元数据（标题 / UP主 / 时长 / 发布时间 / 标签 / 评论 / 弹幕）并让轻量模型写一段摘要，
// 不打开播放器、不下载视频、不写解析文本：
// 这里回答的是“这条视频讲了什么”，是否真的看过仍由生活链的看视频分支决定，
// 避免月华把页面速览说成自己看过视频。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/ViewerScreening"
	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// directImpressionCommentMax 直驱印象采集的评论条数（翻页聚合后去重）
	directImpressionCommentMax = 60
	// directImpressionDanmakuMax 直驱印象采集的弹幕条数（已去重）
	directImpressionDanmakuMax = 80
)

// DirectImpression 指定视频的 URL 直驱印象（规范三的 BilibiliSearch 直驱模式）。
//
// 采集公开元信息（标题 / UP主 / 时长 / 标签 / 简介）以及高赞评论、弹幕（均去重），
// 切换 light_reasoning 写一段视频印象后立即返回；不选片、不看动态、不切片，
// 也不产生任何分片地址或临时地址。评论/弹幕取不到时退回纯元信息。
func DirectImpression(rawURL, instruction string) (string, error) {
	bvid := viewerstore.ExtractBVID(rawURL)
	if bvid == "" {
		return "", fmt.Errorf("不是有效的 B 站视频地址")
	}
	info, err := bilibili.FetchVideo(bvid)
	if err != nil {
		return "", fmt.Errorf("读取视频信息失败: %w", err)
	}
	tags, tagErr := bilibili.FetchTags(bvid)
	if tagErr != nil {
		logger.Warn("BilibiliViewer", "直驱印象读取标签失败（继续）: %v", tagErr)
	}
	// 评论翻页聚合 + 弹幕均按内容去重，失败不影响主材料
	comments := fetchQuickComments(info.Aid, directImpressionCommentMax)
	danmaku := bilibili.FetchDanmaku(info.Cid, directImpressionDanmakuMax)
	material := quickDigestMaterial(info, tags, comments, danmaku, directImpressionCommentMax)
	// 卸主模型 → light_reasoning 写印象 → 收尾卸载
	unloadModel(modelPrimary)
	text, modelErr := ViewerScreening.DirectSummary(context.Background(), material, instruction, modelLight)
	unloadModel(modelLight)
	if modelErr != nil {
		logger.Warn("BilibiliViewer", "直驱印象生成失败，改用材料兜底: %v", modelErr)
		return fallbackQuickDigest(info, tags), nil
	}
	summary := strings.TrimSpace(text)
	if summary == "" {
		summary = fallbackQuickDigest(info, tags)
	}
	return truncate(summary, 1200), nil
}

// fetchQuickComments 公开评论接口读取高赞评论（按内容去重）；失败返回 nil。
//
// 分页与响应解析在 engine/bilibili，这里只保留「去重 + 截断到 max」的业务口径。
func fetchQuickComments(aid int64, max int) []commentItem {
	if aid <= 0 || max <= 0 {
		return nil
	}
	out := make([]commentItem, 0, max)
	seen := make(map[string]bool, max)
	for _, item := range bilibili.HighLikeComments(aid, max) {
		// 去重键：折叠空白并忽略大小写，挡住「第一」「666」这类重复刷屏
		key := commentDedupeKey(item.Content)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= max {
			break
		}
	}
	return out
}

// commentDedupeKey 评论去重键：折叠空白并忽略大小写。
func commentDedupeKey(content string) string {
	return strings.ToLower(strings.Join(strings.Fields(content), " "))
}

// quickDigestMaterial 把采集到的材料排版成轻量模型的输入。
// commentRows 控制评论展示条数（速览只展示少量，直驱印象可覆盖全部采集结果）。
func quickDigestMaterial(info videoInfo, tags []string, comments []commentItem, danmaku []string, commentRows int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "标题：%s\n", info.Title)
	fmt.Fprintf(&builder, "UP主：%s\n", info.Owner)
	if info.Duration > 0 {
		fmt.Fprintf(&builder, "时长：%d 分 %d 秒\n", info.Duration/60, info.Duration%60)
	}
	if info.PublishTime > 0 {
		fmt.Fprintf(&builder, "发布时间：%s\n", time.Unix(info.PublishTime, 0).Format("2006-01-02"))
	}
	if len(tags) > 0 {
		fmt.Fprintf(&builder, "标签：%s\n", strings.Join(tags, "、"))
	}
	if desc := truncate(strings.TrimSpace(info.Desc), 512); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", desc)
	}
	if len(comments) > 0 {
		builder.WriteString("高赞评论：\n")
		for index, item := range comments {
			if commentRows > 0 && index >= commentRows {
				break
			}
			fmt.Fprintf(&builder, "- %s：%s\n", item.User, item.Content)
		}
	}
	if len(danmaku) > 0 {
		fmt.Fprintf(&builder, "弹幕：%s\n", strings.Join(danmaku, " / "))
	}
	return strings.TrimSpace(builder.String())
}

// fallbackQuickDigest 模型不可用时的材料摘要：标题、UP主、时长、标签与简介。
func fallbackQuickDigest(info videoInfo, tags []string) string {
	parts := make([]string, 0, 4)
	head := fmt.Sprintf("《%s》，UP主 %s", info.Title, info.Owner)
	if info.Duration > 0 {
		head += fmt.Sprintf("，时长 %d 分钟", info.Duration/60)
	}
	parts = append(parts, head)
	if len(tags) > 0 {
		parts = append(parts, "标签："+strings.Join(tags, "、"))
	}
	if desc := truncate(strings.TrimSpace(info.Desc), 512); desc != "" {
		parts = append(parts, desc)
	}
	return strings.Join(parts, "；")
}
