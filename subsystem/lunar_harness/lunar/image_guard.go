package lunar

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 模型请求的出站图片闸门（docs/adr/0008） ====
//
// 思维链里图片只允许一种形态：视觉读取管线产出的副本（/file/miniature/ 端点，
// 或 base64 形态下同管线的 data URI）。其余来源一律视为原图：
//   - 远程 URL（QQ 图床等）：模型会自行下载原图，一张 3000×4000 的照片就能
//     吃满多模态上下文，随后每轮模型都只回空内容；
//   - /file/read/ 直读地址、file:// 裸路径：绕过缩放读原图；
//   - 超尺寸内联图。
//
// 拦到就**整条替换为文本占位**：既不让原图进入模型，也不让整轮推理失败。

// hostDerivedImageMaxSide 宿主自产图片（截图、采样帧）的固定上限
const hostDerivedImageMaxSide = 1024

// blockedImagePlaceholder 被拦截图片的替身文本
const blockedImagePlaceholder = "[图片已拦截：思维链只接受视觉读取产物（/file/miniature/ 或视觉副本）]"

// guardRequestImages 逐条检查待发送消息里的 image_url，返回改写后的消息与被拦截说明
func guardRequestImages(messages []PostMessage) ([]PostMessage, []string) {
	sideCap := config.VisionMaxSide
	if sideCap < hostDerivedImageMaxSide {
		sideCap = hostDerivedImageMaxSide
	}
	blocked := make([]string, 0)
	for i := range messages {
		items := asItems(messages[i].Content)
		if len(items) == 0 {
			continue
		}
		changed := false
		for j := range items {
			if items[j].Type != ContentImageURL || items[j].ImageURL == nil {
				continue
			}
			reason := imageBlockReason(items[j].ImageURL.URL, sideCap)
			if reason == "" {
				continue
			}
			blocked = append(blocked, fmt.Sprintf("%s | %s", reason, truncateRunes(items[j].ImageURL.URL, 96)))
			items[j] = ContentItem{Type: ContentText, Text: blockedImagePlaceholder}
			changed = true
		}
		if changed {
			messages[i].Content = items
		}
	}
	if len(blocked) > 0 {
		logger.Warn(lunarCoreModule, "[图片闸门] 已拦截 %d 张非视觉产物图片，改为文本占位", len(blocked))
	}
	return messages, blocked
}

// imageBlockReason 判定 image_url 是否允许进入模型上下文；空串表示放行
func imageBlockReason(raw string, sideCap int) string {
	if raw == "" {
		return "空图片地址"
	}
	if isLocalVisionURL(raw) {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:video/") || hasVideoExt(lower) {
		return ""
	}
	if strings.HasPrefix(lower, "data:image/") {
		width, height, err := inlineImageSize(raw)
		if err != nil {
			return "内联图无法解析尺寸"
		}
		if width > sideCap || height > sideCap {
			return fmt.Sprintf("内联图 %dx%d 超过 %d 上限", width, height, sideCap)
		}
		return ""
	}
	return "非视觉端点来源"
}

// isLocalVisionURL 判断地址是否为本机 /file/miniature/ 端点
func isLocalVisionURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return false
	}
	if !strings.HasPrefix(parsed.Path, visionReadPath) {
		return false
	}
	return parsed.Port() != "" && parsed.Port() == strconv.Itoa(config.BasicPort)
}

// inlineImageSize 从 data:image/*;base64 中读取像素尺寸（只解码文件头）
func inlineImageSize(raw string) (int, int, error) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return 0, 0, fmt.Errorf("data URI 缺少逗号分隔")
	}
	header := strings.ToLower(raw[:comma])
	if !strings.Contains(header, ";base64") {
		return 0, 0, fmt.Errorf("data URI 非 base64 编码")
	}
	cfg, _, err := image.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(raw[comma+1:])))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
