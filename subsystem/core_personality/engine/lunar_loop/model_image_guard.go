package lunarloop

// model_image_guard.go 模型请求的出站图片闸门
//
// 思维链里的图片只允许一种形态：视觉读取管线（/file/miniature/ 端点或
// visionRead → VisionCopy）产出的副本。其余一切来源都视为「原图」：
//
//   - 远程 URL（QQ 图床等）：模型会自己去下载原图；
//   - file:// 裸路径：绕过缩放直接读盘；
//   - 内联 data URI：允许视觉读取管线产出的 base64 副本（≤ vision_max_side）与
//     宿主自产缩略图；超尺寸内联图视为原图；
//   - 任何超尺寸内联图。
//
// 所有模型调用都经 syncFetch → /chat/completions，这里在真正发请求前逐条检查
// messages[].content[].image_url：不合规的**整条替换为文本占位**（不是放行、不是
// 截断），保证「模型永远拿不到原图数据」。放行标准见 docs/adr/0008。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strconv"
	"strings"

	"LunarSubsystem/GeneralConfig"
)

// 宿主自产图片（屏幕截图 ≤1024、视频采样帧 ≤640）的硬上限。
// 内联图的上限取 max(vision_max_side, 本值)：视觉副本受配置约束，宿主自产件按固定上限，
// 两者都不能突破「不是原图」这条线。
const hostDerivedImageMaxSide = 1024

// 被拦截图片的替身文本（进入模型上下文，措辞保持简短稳定）
const blockedImagePlaceholder = "[图片已拦截：思维链只接受视觉读取产物（/file/miniature/ 或视觉副本）]"

// 视频类引用白名单：视频不走图片闸门（服务端按 --video-fps 抽帧，另有尺寸约束）
var modelVideoExts = []string{".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v", ".mpg", ".mpeg", ".wmv", ".flv"}

// modelImageSideCap 当前允许进入模型上下文的内联图长边上限（像素）
func modelImageSideCap() int {
	side := *GeneralConfig.VisionMaxSide
	if side < hostDerivedImageMaxSide {
		side = hostDerivedImageMaxSide
	}
	return side
}

// guardChatRequestImages 在模型请求出站前执行图片闸门
//
// 只处理 /chat/completions；命中拦截时把改写后的请求体写回 execute，
// 返回被拦截条目的描述（供调用方落日志）。
func guardChatRequestImages(requestURL string, execute map[string]any) ([]string, error) {
	if !strings.Contains(requestURL, "/chat/completions") {
		return nil, nil
	}
	bodyVal, ok := execute["body"]
	if !ok {
		return nil, nil
	}
	switch v := bodyVal.(type) {
	case map[string]any:
		return guardModelImages(v), nil
	case string:
		// 快路径：请求体里没有 image_url 就不解析（文字请求占绝大多数）
		if !strings.Contains(v, `"image_url"`) {
			return nil, nil
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			// 解析不了就无法证明里面没有原图：放行会让闸门形同虚设，直接拒绝
			return nil, fmt.Errorf("模型请求体解析失败，无法执行图片闸门: %w", err)
		}
		blocked := guardModelImages(obj)
		if len(blocked) == 0 {
			return nil, nil
		}
		rebuilt, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("图片闸门改写请求体失败: %w", err)
		}
		execute["body"] = string(rebuilt)
		return blocked, nil
	}
	return nil, nil
}

// guardModelImages 逐条检查 messages[].content[].image_url，返回被拦截条目的描述
//
// 不合规的图片项被原位替换为文本占位：既不向模型泄漏原图，也不让整轮推理失败。
func guardModelImages(body map[string]any) []string {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil
	}
	sideCap := modelImageSideCap()
	var blocked []string
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		items, ok := message["content"].([]any)
		if !ok {
			continue
		}
		for i, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if kind, _ := item["type"].(string); kind != "image_url" {
				continue
			}
			payload, _ := item["image_url"].(map[string]any)
			raw, _ := payload["url"].(string)
			reason := imageURLBlockReason(raw, sideCap)
			if reason == "" {
				continue
			}
			blocked = append(blocked, fmt.Sprintf("%s | %s", reason, truncateForLog(raw, 96)))
			items[i] = map[string]any{"type": "text", "text": blockedImagePlaceholder}
		}
	}
	return blocked
}

// imageURLBlockReason 判定一个 image_url 是否允许进入模型上下文；空串表示放行
func imageURLBlockReason(raw string, sideCap int) string {
	if raw == "" {
		return "空图片地址"
	}
	// 静态图合规形态一：本机视觉读取端点（缩放 + 叠印已在服务端完成）
	if isLocalVisionURL(raw) {
		return ""
	}
	lower := strings.ToLower(raw)
	// 视频引用不属图片闸门管辖（抽帧频率与尺寸另有约束）
	if strings.HasPrefix(lower, "data:video/") || hasVideoExt(lower) {
		return ""
	}
	// 静态图合规形态二：视觉读取管线产出的内联副本 / 宿主自产缩略图（受尺寸上限约束）
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
//
// 用「路径前缀 + 本机端口」判定，不绑定主机名：服务在 allow_lan 打开时给出的是
// 局域网 IP，但端口恒为基本端口，路径恒为视觉读取前缀。
func isLocalVisionURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return false
	}
	if !strings.HasPrefix(parsed.Path, "/file/miniature/") {
		return false
	}
	return parsed.Port() != "" && parsed.Port() == strconv.Itoa(*GeneralConfig.BasicPort)
}

// inlineImageSize 从 data:image/*;base64 里读出像素尺寸（只解码文件头）
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

// hasVideoExt 判断地址（去查询串）是否以视频扩展名结尾
func hasVideoExt(lower string) bool {
	path := lower
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	for _, ext := range modelVideoExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// truncateForLog 截断日志里的地址，避免把整段 base64 写进日志
func truncateForLog(raw string, limit int) string {
	if len(raw) <= limit {
		return raw
	}
	return raw[:limit] + "…（共 " + strconv.Itoa(len(raw)) + " 字符）"
}
