package BilibiliLife

// sideinfo.go 侧面理解：某条视频的画面切片触发思考监管重复时，
// 放弃全部切片理解，改用标题 + 封面 + 简介 + 热门评论 + 弹幕推测视频内容，
// 为后续的评论、日记动态与分享评论提供依据。

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// danmakuRe 弹幕 XML 里的单条内容：<d p="...">文本</d>
var danmakuRe = regexp.MustCompile(`(?s)<d [^>]*>(.*?)</d>`)

// fetchDanmaku 读取视频弹幕（最多 max 条）；失败只告警，不阻断侧面理解。
func fetchDanmaku(cid int64, max int) []string {
	if cid <= 0 || max <= 0 {
		return nil
	}
	target := fmt.Sprintf("https://comment.bilibili.com/%d.xml", cid)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", biliReferer)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("BilibiliLife", "读取弹幕失败（侧面理解只用其余信息）: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	out := make([]string, 0, max)
	seen := make(map[string]bool, max)
	for _, match := range danmakuRe.FindAllStringSubmatch(string(body), -1) {
		text := strings.TrimSpace(html.UnescapeString(match[1]))
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, truncate(text, 60))
		if len(out) >= max {
			break
		}
	}
	return out
}

// fetchCoverDataURL 下载封面并转成 data URI；失败返回空串（侧面理解退回纯文字材料）。
// B 站封面地址常带 @ 缩放后缀，去掉后取原图；只接受 jpg/png，避免模型端不支持 webp。
func fetchCoverDataURL(pic string) string {
	pic = strings.TrimSpace(pic)
	if pic == "" {
		return ""
	}
	if idx := strings.Index(pic, "@"); idx > 0 {
		pic = pic[:idx]
	}
	req, err := http.NewRequest(http.MethodGet, pic, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", biliReferer)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("BilibiliLife", "下载视频封面失败（侧面理解只用文字）: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(contentType, "image/jpeg") && !strings.Contains(contentType, "image/png") {
		return ""
	}
	if strings.Contains(contentType, "jpeg") {
		contentType = "image/jpeg"
	} else {
		contentType = "image/png"
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, fallbackCoverMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > fallbackCoverMaxBytes {
		return ""
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// composeSideUnderstanding 构建侧面理解：优先让模型结合封面与文字材料归纳，
// 失败（含模型再次陷入重复）时退回纯文字拼装，保证下游写作始终有依据。
func composeSideUnderstanding(info videoInfo, comments []commentItem, danmaku []string, cover string) string {
	material := sideMaterial(info, comments, danmaku)
	task := "这个视频的画面理解失败了（模型在画面描述时反复陷入重复输出），你只能依据标题、封面、简介、热门评论和弹幕推测它大致讲了什么、观众的情绪如何。" +
		"写 3–6 行侧面理解，供之后写评论、日记和分享使用；不确定的地方用「看起来/大概是」表述，不要编造具体情节。只输出正文。"
	if system, err := lifeSystem(task); err == nil {
		var text string
		var chatErr error
		if cover != "" {
			text, chatErr = chatWithImages(system, material, []string{cover})
		} else {
			text, chatErr = chatText(system, material)
		}
		if text = strings.TrimSpace(text); chatErr == nil && text != "" {
			return truncate(text, 900)
		}
		logger.Warn("BilibiliLife", "侧面理解归纳失败，改用纯文字材料: %v", chatErr)
	}
	return truncate(material, 900)
}

// sideMaterial 汇总侧面信息（模型归纳与纯文字兜底共用）
func sideMaterial(info videoInfo, comments []commentItem, danmaku []string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "视频：《%s》\nUP主：%s\n", info.Title, info.Owner)
	if desc := strings.TrimSpace(info.Desc); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", truncate(desc, 600))
	}
	if len(comments) > 0 {
		builder.WriteString("热门评论：\n")
		for _, comment := range comments {
			fmt.Fprintf(&builder, "%s：%s\n", comment.User, truncate(comment.Content, 120))
		}
	}
	if len(danmaku) > 0 {
		builder.WriteString("弹幕：")
		builder.WriteString(strings.Join(danmaku, " / "))
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
