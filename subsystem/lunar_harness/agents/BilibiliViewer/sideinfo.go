package BilibiliViewer

// sideinfo.go 视频自身信息的采集与轻量印象：封面转码、弹幕、以及只看标题/标签/
// 简介/封面时对视频的初步判断。
//
// 分层约定：
//   - 视频印象（composeVideoImpression）只用视频自身信息 + 封面，不注入长期记忆；
//   - 评论/分享写作（run.go）在此基础上再叠加评论区与弹幕作为参考，同样不注入记忆；
//   - 长期记忆只用于回复消息与发动态。

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"

	_ "golang.org/x/image/webp"
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
		logger.Warn("BilibiliViewer", "读取弹幕失败（侧面理解只用其余信息）: %v", err)
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

// fetchCoverDataURL 取封面并统一转码成小尺寸 JPEG data URI。
//
// B 站封面原图常见 webp/avif、尺寸动辄数 MB；原样 base64 塞进多模态请求时，
// 请求体可达数 MB，本地/中转模型端点容易直接返回 500。这里复用 multimodal
// 已有的解码与缩放能力（ToRGBA / ResizeToFit），把封面统一收敛为：
// 长边 ≤ coverMaxDim 的 JPEG。格式与体积都稳定，模型端不会再因图片报错。
//
// 优先让 CDN 直接输出缩略图（@480w_270h.jpg），失败再取原图自行转码。
func fetchCoverDataURL(pic string) string {
	base := strings.TrimSpace(pic)
	if base == "" {
		return ""
	}
	// B 站图片地址可能自带 @ 缩放后缀，先裁掉，再按需要重新拼
	if idx := strings.Index(base, "@"); idx > 0 {
		base = base[:idx]
	}
	candidates := []string{base + "@480w_270h.jpg", base}
	for _, candidate := range candidates {
		if data := downloadCoverJPEG(candidate); data != "" {
			return data
		}
	}
	return ""
}

// downloadCoverJPEG 下载一张图片并转成规范化的 JPEG data URI；任何一步失败返回空串。
func downloadCoverJPEG(target string) string {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", biliReferer)
	// 明确只要位图格式，避免 CDN 按 Accept 协商返回 webp/avif
	req.Header.Set("Accept", "image/jpeg,image/png,image/*;q=0.8")
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("BilibiliViewer", "下载视频封面失败（改用纯文字材料）: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, fallbackCoverMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > fallbackCoverMaxBytes {
		return ""
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		logger.Warn("BilibiliViewer", "封面解码失败（改用纯文字材料）: %v", err)
		return ""
	}
	// 复用 multimodal 的图元转换与等比缩放，统一控制长边尺寸
	rgba := multimodal.ResizeToFit(multimodal.ToRGBA(img), coverMaxDim, coverMaxDim)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: coverJPEGQuality}); err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
