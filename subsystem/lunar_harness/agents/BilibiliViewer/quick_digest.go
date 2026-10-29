package BilibiliViewer

// quick_digest.go 指定视频的快速内容速览。
//
// 只聚合公开元数据（标题 / UP主 / 时长 / 发布时间 / 标签 / 评论 / 弹幕）并让轻量模型写一段摘要，
// 不打开播放器、不下载视频、不写解析文本：
// 这里回答的是“这条视频讲了什么”，是否真的看过仍由正式观影流程决定，
// 避免月华把页面速览说成自己看过视频。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// quickDigestMaxRunes 速览摘要的最大字数（要直接进对话上下文，不能太长）
	quickDigestMaxRunes = 320
	// quickDigestCommentMax 速览采集的高赞评论条数
	quickDigestCommentMax = 20
	// quickDigestDanmakuMax 速览采集的弹幕条数
	quickDigestDanmakuMax = 20
)

// QuickDigest 指定 B 站视频的快速速览：返回标题与摘要。
//
// 只使用公开接口，不依赖浏览器登录态；评论或弹幕取不到时用其余材料继续，
// 素材不足时退化为“标题 + UP主 + 时长 + 标签 + 简介”的可读摘要。
func QuickDigest(rawURL, instruction string) (title, summary string, err error) {
	bvid := bvidRe.FindString(rawURL)
	if bvid == "" {
		return "", "", fmt.Errorf("不是有效的 B 站视频地址")
	}
	info, err := fetchVideoInfo(bvid)
	if err != nil {
		return "", "", fmt.Errorf("读取视频信息失败: %w", err)
	}
	title = strings.TrimSpace(info.Title)
	if title == "" {
		title = bvid
	}

	// 标签 / 评论 / 弹幕都属“能取到就补充”的材料，单项失败不影响速览。
	tags, tagErr := fetchVideoTags(bvid)
	if tagErr != nil {
		logger.Warn("BilibiliViewer", "速览读取标签失败（继续）: %v", tagErr)
	}
	comments := fetchQuickComments(info.Aid, quickDigestCommentMax)
	danmaku := fetchDanmaku(info.Cid, quickDigestDanmakuMax)

	material := quickDigestMaterial(info, tags, comments, danmaku)
	task := "你是 B 站视频速览助手。根据标题、UP主、时长、发布时间、标签、简介、评论与弹幕，" +
		"写 2-4 句中文速览：先说明这条视频大概讲什么，再补充观众关注点或氛围。" +
		"只依据材料，不编造具体情节，不写“我认为/推荐”。"
	if strings.TrimSpace(instruction) != "" {
		task += "本次特别关注：" + truncate(instruction, 80) + "。"
	}

	// 与观影流程同一套模型生命周期：先让出主模型，用轻量模型写速览，写完卸掉。
	unloadModel(modelPrimary)
	text, modelErr := chatTextModel(modelLight, task, material)
	unloadModel(modelLight)
	if modelErr != nil {
		logger.Warn("BilibiliViewer", "速览摘要生成失败，改用材料兜底: %v", modelErr)
		return title, fallbackQuickDigest(info, tags), nil
	}
	summary = strings.TrimSpace(text)
	if summary == "" {
		summary = fallbackQuickDigest(info, tags)
	}
	return title, truncate(summary, quickDigestMaxRunes), nil
}

// DirectImpression 指定视频的 URL 直驱印象（规范三的 BilibiliSearch 直驱模式）。
//
// 只取公开元信息（标题 / UP主 / 时长 / 标签 / 简介），切换 light_reasoning 写一段视频印象后立即返回；
// 不选片、不看动态、不读评论、不切片，也不产生任何分片地址或临时地址。
func DirectImpression(rawURL, instruction string) (string, error) {
	bvid := bvidRe.FindString(rawURL)
	if bvid == "" {
		return "", fmt.Errorf("不是有效的 B 站视频地址")
	}
	info, err := fetchVideoInfo(bvid)
	if err != nil {
		return "", fmt.Errorf("读取视频信息失败: %w", err)
	}
	tags, tagErr := fetchVideoTags(bvid)
	if tagErr != nil {
		logger.Warn("BilibiliViewer", "直驱印象读取标签失败（继续）: %v", tagErr)
	}
	material := quickDigestMaterial(info, tags, nil, nil)
	task := "你是视频印象整理者。根据标题、UP主、时长、发布时间、标签与简介，" +
		"写一段 120-220 字的第一人称视频印象：先说明这条视频大概讲什么，再补充风格、节奏与可能的看点。" +
		"只依据材料，不编造具体情节，不写“我认为/推荐”。"
	if strings.TrimSpace(instruction) != "" {
		task += "本次特别关注：" + truncate(instruction, 80) + "。"
	}
	// 卸主模型 → light_reasoning 写印象 → 收尾卸载
	unloadModel(modelPrimary)
	text, modelErr := chatTextModel(modelLight, task, material)
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

// fetchQuickComments 公开评论接口读取首屏高赞评论；失败返回 nil。
func fetchQuickComments(aid int64, max int) []commentItem {
	if aid <= 0 || max <= 0 {
		return nil
	}
	target := fmt.Sprintf("https://api.bilibili.com/x/v2/reply?type=1&oid=%d&sort=2&ps=%d&pn=1", aid, max)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Member struct {
					Uname string `json:"uname"`
				} `json:"member"`
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
				Like int `json:"like"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := biliHTTPGetJSON(target, &resp); err != nil {
		logger.Warn("BilibiliViewer", "速览读取评论失败（继续）: %v", err)
		return nil
	}
	if resp.Code != 0 {
		logger.Warn("BilibiliViewer", "速览评论接口 code=%d", resp.Code)
		return nil
	}
	out := make([]commentItem, 0, len(resp.Data.Replies))
	for _, item := range resp.Data.Replies {
		content := strings.TrimSpace(item.Content.Message)
		if content == "" {
			continue
		}
		out = append(out, commentItem{
			User:    strings.TrimSpace(item.Member.Uname),
			Content: truncate(content, 100),
			Like:    item.Like,
		})
	}
	return out
}

// quickDigestMaterial 把采集到的材料排版成轻量模型的输入。
func quickDigestMaterial(info videoInfo, tags []string, comments []commentItem, danmaku []string) string {
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
	if desc := truncate(strings.TrimSpace(info.Desc), 300); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", desc)
	}
	if len(comments) > 0 {
		builder.WriteString("高赞评论：\n")
		for index, item := range comments {
			if index >= 8 {
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
	if desc := truncate(strings.TrimSpace(info.Desc), 120); desc != "" {
		parts = append(parts, desc)
	}
	return strings.Join(parts, "；")
}
