package Screenshoter

// capture.go 屏幕/窗口取景、截图、落盘与汇总。

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// executeTool 执行一次截图工具调用。
func (s *session) executeTool(call toolCall) string {
	args, _ := agentbase.ParseToolArgs(call.Arguments)
	if args == nil {
		args = map[string]any{}
	}
	switch call.Name {
	case "list_capture_targets":
		return describeCaptureTargets(args)
	case "capture_screen":
		text, item := s.captureScreen(args)
		s.pending = item
		return text
	default:
		return "未知工具: " + call.Name + "，可用工具为 list_capture_targets、capture_screen"
	}
}

// describeCaptureTargets 列出显示器与可见窗口。
func describeCaptureTargets(args map[string]any) string {
	includeMinimized := boolArg(args, "include_minimized")
	sections := make([]string, 0)
	displays := multimodal.GetDisplays()
	if len(displays) > 0 {
		sections = append(sections, fmt.Sprintf("显示器 %d 个：", len(displays)))
		for _, display := range displays {
			sections = append(sections, fmt.Sprintf("- index=%d｜%dx%d｜屏幕原点 (%d, %d)", display["index"], display["width"], display["height"], display["x"], display["y"]))
		}
	}
	windows, err := multimodal.ListWindows()
	if err != nil {
		sections = append(sections, "窗口枚举失败："+err.Error())
	} else {
		visible := make([]multimodal.WindowInfo, 0, len(windows))
		for _, item := range windows {
			if includeMinimized || !item.Minimized {
				visible = append(visible, item)
			}
		}
		if len(visible) == 0 {
			sections = append(sections, "没有可截图的可见窗口（可用 mode=fullscreen 截整屏）。")
		} else {
			sections = append(sections, fmt.Sprintf("可见窗口 %d 个：", len(visible)))
			shown := visible
			if len(shown) > 40 {
				shown = shown[:40]
			}
			for _, item := range shown {
				flags := make([]string, 0, 2)
				if item.Minimized {
					flags = append(flags, "最小化")
				}
				if item.Foreground {
					flags = append(flags, "焦点")
				}
				process := ""
				if item.Process != "" {
					process = "（" + item.Process + "）"
				}
				line := fmt.Sprintf("- handle=%d｜%s%s｜窗口 %dx%d｜客户区 %dx%d", item.Handle, item.Title, process, item.Width, item.Height, item.ClientW, item.ClientH)
				if len(flags) > 0 {
					line += "｜" + strings.Join(flags, "｜")
				}
				sections = append(sections, line)
			}
			if len(visible) > len(shown) {
				sections = append(sections, fmt.Sprintf("……其余 %d 个窗口未列出。", len(visible)-len(shown)))
			}
		}
	}
	sections = append(sections, "提示：按标题截图用 window_title（包含匹配），需要精确锁定用 window_handle；最小化窗口需先恢复才能截取。")
	return strings.Join(sections, "\n")
}

// buildCaptureRequest 把工具参数映射为引擎截图请求。
func buildCaptureRequest(args map[string]any) multimodal.CaptureRequest {
	request := multimodal.CaptureRequest{Mode: multimodal.ModeAuto, Format: "png"}
	if mode := stringArg(args, "mode"); mode != "" {
		request.Mode = multimodal.CaptureMode(mode)
	}
	request.DisplayIndex = int(numberArg(args, "display_index"))
	request.WindowHandle = uint64(numberArg(args, "window_handle"))
	request.WindowTitle = strings.TrimSpace(stringArg(args, "window_title"))
	request.ClientArea = boolArg(args, "client_area")
	request.OffsetX = int(numberArg(args, "offset_x"))
	request.OffsetY = int(numberArg(args, "offset_y"))
	request.Width = int(numberArg(args, "width"))
	request.Height = int(numberArg(args, "height"))
	request.RegionX = int(numberArg(args, "region_x"))
	request.RegionY = int(numberArg(args, "region_y"))
	request.RegionW = int(numberArg(args, "region_w"))
	request.RegionH = int(numberArg(args, "region_h"))
	if format := stringArg(args, "format"); format != "" {
		request.Format = format
	}
	return request
}

// captureScreen 执行截图、推送并返回主帧。
func (s *session) captureScreen(args map[string]any) (string, *shot) {
	request := buildCaptureRequest(args)
	note := strings.TrimSpace(stringArg(args, "note"))
	result, err := multimodal.Capture(request)
	if err != nil {
		return "截图失败：" + err.Error(), nil
	}
	frames, err := multimodal.ResizeImage(result.Image)
	if err != nil || len(frames) == 0 {
		return "截图失败：未获取到图像数据", nil
	}
	urls := make([]string, 0, len(frames))
	for index, frame := range frames {
		url := s.persistShot(frame, index)
		if url != "" {
			urls = append(urls, url)
		}
	}
	if len(urls) > 0 {
		s.env.PushImages(urls)
	}
	first := frames[0]
	modeText := defaultString(stringArg(args, "mode"), string(result.Mode))
	windowText := ""
	if result.WindowTitle != "" {
		windowText = "，窗口「" + result.WindowTitle + "」"
	}
	sourceSize := ""
	if result.Width > 0 && result.Height > 0 {
		sourceSize = fmt.Sprintf("，原图 %dx%d", result.Width, result.Height)
	}
	frameText := ""
	if len(frames) > 1 {
		frameText = fmt.Sprintf("，共 %d 帧", len(frames))
	}
	label := ""
	if note != "" {
		label = note + "："
	}
	width := toIntValue(first["width"])
	height := toIntValue(first["height"])
	text := fmt.Sprintf("截图完成。%s模式=%s%s，输出 %dx%d%s%s，图片已展示给用户。", label, modeText, windowText, width, height, sourceSize, frameText)
	shotNote := note
	if shotNote == "" {
		shotNote = modeText + " 截图"
	}
	item := &shot{
		Note: shotNote, Mode: modeText, WindowTitle: result.WindowTitle,
		Width: width, Height: height, Base64: firstBase64(first), URL: firstURL(urls),
	}
	return text, item
}

// persistShot 落盘一帧截图并返回可查看地址。
func (s *session) persistShot(frame map[string]any, index int) string {
	base64Text, _ := frame["base64"].(string)
	if base64Text == "" {
		return ""
	}
	payload := base64Text
	if marker := strings.Index(payload, "base64,"); marker >= 0 {
		payload = payload[marker+len("base64,"):]
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	format, _ := frame["format"].(string)
	ext := "png"
	if format == "jpg" || format == "jpeg" {
		ext = "jpg"
	}
	name := fmt.Sprintf("multimedia/screenshot_%d_%d.%s", time.Now().UnixMilli(), index, ext)
	url, err := s.env.SaveImage(name, data)
	if err != nil {
		logger.Warn(module, "[截图者] 落盘失败: %v", err)
		return ""
	}
	return url
}

// firstBase64 取第一帧的 base64。
func firstBase64(frame map[string]any) string {
	value, _ := frame["base64"].(string)
	return value
}

// firstURL 取第一个可查看地址。
func firstURL(urls []string) string {
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

// buildScreenshotSummary 汇总截图结果。
func buildScreenshotSummary(shots []shot) string {
	if len(shots) == 0 {
		return ""
	}
	primary := shots[len(shots)-1]
	target := "屏幕"
	if primary.WindowTitle != "" {
		target = "「" + primary.WindowTitle + "」"
	}
	size := fmt.Sprintf("%dx%d", primary.Width, primary.Height)
	if len(shots) == 1 {
		return fmt.Sprintf("截好了，%s（%s，%s），已经给你看到了。", primary.Note, target, size)
	}
	return fmt.Sprintf("截好了，一共 %d 张，最后一张是%s（%s，%s），都已经给你看到了。", len(shots), primary.Note, target, size)
}
