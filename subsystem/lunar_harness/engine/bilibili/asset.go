package bilibili

// asset.go 视频自身的辅助信息：弹幕 XML 与封面图。
//
// 封面原图常见 webp/avif、尺寸动辄数 MB；原样 base64 塞进多模态请求时，
// 请求体可达数 MB，本地/中转模型端点容易直接返回 500。这里复用 multimodal
// 已有的解码与缩放能力（ToRGBA / ResizeToFit），把封面统一收敛为
// 长边 ≤ coverMaxDim 的 JPEG，格式与体积都稳定。

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

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"

	_ "golang.org/x/image/webp"
)

// danmakuRe 弹幕 XML 里的单条内容：<d p="...">文本</d>
var danmakuRe = regexp.MustCompile(`(?s)<d [^>]*>(.*?)</d>`)

// FetchDanmaku 读取视频弹幕（最多 max 条）；失败只告警，不阻断调用方的侧面理解。
func FetchDanmaku(cid int64, max int) []string {
	if cid <= 0 || max <= 0 {
		return nil
	}
	target := fmt.Sprintf(danmakuURL, cid)
	req, err := newRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	resp, err := danmakuClient.Do(req)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取弹幕失败（侧面理解只用其余信息）: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDanmakuBytes))
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
		out = append(out, truncateRunes(text, commentMessageMaxRunes))
		if len(out) >= max {
			break
		}
	}
	return out
}

// FetchCoverDataURL 取封面并统一转码成小尺寸 JPEG data URI。
//
// 优先让 CDN 直接输出缩略图（@480w_270h.jpg），失败再取原图自行转码。
func FetchCoverDataURL(pic string) string {
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
	req, err := newRequest(http.MethodGet, target, nil)
	if err != nil {
		return ""
	}
	// 明确只要位图格式，避免 CDN 按 Accept 协商返回 webp/avif
	req.Header.Set("Accept", "image/jpeg,image/png,image/*;q=0.8")
	resp, err := apiClient.Do(req)
	if err != nil {
		logger.Warn("BilibiliViewer", "下载视频封面失败（改用纯文字材料）: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxCoverBytes {
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
