package viewer

// preselect.go 候选视频的印象材料：采集信息标准化排版后交给阅片智能体归纳。
//
// 待看列表补齐管线（watchlist*.go）复用这里的印象生成：排版材料、调用轻量模型、
// 模型不可用时用标题/标签/简介兜底。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/viewer/ViewerScreening"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

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

// buildOneImpression 单条视频印象：材料标准化排版后交给阅片智能体归纳。
func buildOneImpression(video *videoRecord) videoImpression {
	material := impressionMaterial(video)
	impression, err := ViewerScreening.Impress(context.Background(), material, modelLight)
	if err != nil {
		logger.Warn("BilibiliViewer", "《%s》印象生成失败，改用采集信息兜底: %v", video.Info.Title, err)
		return fallbackImpression(video)
	}
	return videoImpression{
		Summary:    impression.Summary,
		Topics:     impression.Topics,
		Tone:       impression.Tone,
		Risky:      impression.Risky,
		RiskReason: impression.RiskReason,
	}
}

// fallbackImpression 模型不可用时的印象兜底：标题 + 标签 + 简介摘要。
func fallbackImpression(video *videoRecord) videoImpression {
	summary := fmt.Sprintf("《%s》，UP主 %s。%s", video.Info.Title, video.Info.Owner,
		truncate(strings.TrimSpace(video.Info.Desc), 120))
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
	if video.Info.OwnerMid > 0 {
		fmt.Fprintf(&builder, "UP主UID：%d\n", video.Info.OwnerMid)
	}
	fmt.Fprintf(&builder, "时长：%d 秒\n", video.Info.Duration)
	if video.Info.PublishTime > 0 {
		fmt.Fprintf(&builder, "发布时间：%s\n", time.Unix(video.Info.PublishTime, 0).Format("2006-01-02"))
	}
	if len(video.Tags) > 0 {
		fmt.Fprintf(&builder, "标签：%s\n", strings.Join(video.Tags, "、"))
	}
	if desc := strings.TrimSpace(video.Info.Desc); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", truncate(desc, 600))
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
