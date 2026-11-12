package viewer

// preselect.go 候选去重与观看期「视频印象」的材料排版。
//
// 观看阶段把采集到的标题、评论、标签、简介、UP 主，加上观看时补取的弹幕与
// 这一分钟的画面理解，统一排版成一份材料，交给阅片智能体合成视频印象。

import (
	"fmt"
	"strings"
	"time"
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

// impressionMaterial 把视频元信息、评论、弹幕与随机一分钟的画面理解排成视频印象材料。
func impressionMaterial(video *videoRecord, visual string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "《%s》\n", video.Info.Title)
	if owner := strings.TrimSpace(video.Info.Owner); owner != "" {
		fmt.Fprintf(&builder, "UP主：%s\n", owner)
	}
	if video.Info.Duration > 0 {
		fmt.Fprintf(&builder, "时长：%s\n", formatDuration(video.Info.Duration))
	}
	if video.Info.PublishTime > 0 {
		fmt.Fprintf(&builder, "发布时间：%s\n", time.Unix(video.Info.PublishTime, 0).Format("2006-01-02"))
	}
	if len(video.Tags) > 0 {
		fmt.Fprintf(&builder, "标签：%s\n", strings.Join(video.Tags, "、"))
	}
	if desc := strings.TrimSpace(video.Info.Desc); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", truncate(desc, watchImpressionIntroRunes))
	}
	if video.Info.View > 0 || video.Info.Like > 0 || video.Info.Favorite > 0 || video.Info.ReplyNum > 0 || video.Info.Danmaku > 0 {
		fmt.Fprintf(&builder, "播放：%d｜点赞：%d｜收藏：%d｜评论：%d｜弹幕：%d\n",
			video.Info.View, video.Info.Like, video.Info.Favorite, video.Info.ReplyNum, video.Info.Danmaku)
	}
	if len(video.Comments) > 0 {
		builder.WriteString("评论：\n")
		for index, comment := range video.Comments {
			if index >= watchImpressionCommentRows {
				break
			}
			if user := strings.TrimSpace(comment.User); user != "" {
				fmt.Fprintf(&builder, "- %s：%s\n", user, comment.Content)
			} else {
				fmt.Fprintf(&builder, "- %s\n", comment.Content)
			}
		}
	}
	if len(video.Danmaku) > 0 {
		sample := video.Danmaku
		if len(sample) > watchImpressionDanmakuRows {
			sample = sample[:watchImpressionDanmakuRows]
		}
		fmt.Fprintf(&builder, "弹幕：%s\n", strings.Join(sample, " / "))
	}
	if visual = strings.TrimSpace(visual); visual != "" {
		fmt.Fprintf(&builder, "随机一分钟画面：%s\n", truncate(visual, watchImpressionVisualRunes))
	}
	return strings.TrimSpace(builder.String())
}

// formatDuration 把秒数格式化成「x分y秒」，不足一分钟只显示秒。
func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "未知"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := seconds / 60
	rest := seconds % 60
	if rest == 0 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 分 %d 秒", minutes, rest)
}
