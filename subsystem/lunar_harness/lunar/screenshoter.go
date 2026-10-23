package lunar

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
)

// newScreenshoterRole 创建截图者角色
func newScreenshoterRole() *ScreenshoterRole {
	builder := newModelBuilder(readPrompt("prompts/screenshoterRole.md"), "截图者", 40)
	return &ScreenshoterRole{ModelBuilder: builder, MaxIterations: 4}
}

func (role *ScreenshoterRole) createCreativeWork(taskDescription string) string {
	role.PendingShot = nil
	role.PrimaryImage = ""
	shots := make([]ScreenshotShot, 0)
	return runCreativeLoop("截图者", role.ModelBuilder, taskDescription, screenshoterToolDefinitions(), role.MaxIterations,
		func(call ToolCallItem) string { return role.executeTool(call) },
		func(call ToolCallItem) {
			if role.PendingShot != nil {
				shots = append(shots, *role.PendingShot)
				role.PendingShot = nil
			}
		},
		func() string {
			if len(shots) == 0 {
				return ""
			}
			primary := shots[len(shots)-1]
			role.PrimaryImage = primary.Base64
			return buildScreenshotSummary(shots)
		},
	)
}

func (role *ScreenshoterRole) executeTool(call ToolCallItem) string {
	args := parseToolArgs(call.Function.Arguments)
	switch call.Function.Name {
	case "list_capture_targets":
		return describeCaptureTargets(args)
	case "capture_screen":
		text, shot := captureScreen(args)
		role.PendingShot = shot
		return text
	default:
		return "未知工具: " + call.Function.Name + "，可用工具为 list_capture_targets、capture_screen"
	}
}

func (role *ScreenshoterRole) takePrimaryImage() string {
	image := role.PrimaryImage
	role.PrimaryImage = ""
	return image
}

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

func captureScreen(args map[string]any) (string, *ScreenshotShot) {
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
		url := persistShot(frame, index)
		if url != "" {
			urls = append(urls, url)
		}
	}
	if len(urls) > 0 {
		pushAssistantImage(urls, false)
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
	shot := &ScreenshotShot{
		Note: shotNote, Mode: modeText, WindowTitle: result.WindowTitle,
		Width: width, Height: height, Base64: firstBase64(first), URL: firstURL(urls),
	}
	return text, shot
}

func persistShot(frame map[string]any, index int) string {
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
	if _, err := persistenceSaveBytes(name, data); err != nil {
		logger.Warn(lunarCoreModule, "[截图者] 落盘失败: %v", err)
		return ""
	}
	return localPathToReadURL(name)
}

func firstBase64(frame map[string]any) string {
	value, _ := frame["base64"].(string)
	return value
}

func firstURL(urls []string) string {
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func buildScreenshotSummary(shots []ScreenshotShot) string {
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

func screenshoterToolDefinitions() []ToolCall {
	return []ToolCall{
		functionTool("list_capture_targets",
			"列出当前所有显示器与可见窗口（标题、进程、窗口矩形、客户区尺寸、是否最小化、是否焦点）。",
			map[string]any{"include_minimized": map[string]any{"type": "boolean", "description": "是否也列出已最小化的窗口"}},
			[]string{}),
		functionTool("capture_screen",
			"执行一次截图并立即展示给用户。mode=auto/window/fullscreen/display/region。",
			map[string]any{
				"mode":          map[string]any{"type": "string", "enum": []string{"auto", "window", "fullscreen", "display", "region"}, "description": "截图模式，默认 auto"},
				"window_title":  map[string]any{"type": "string", "description": "目标窗口标题关键字"},
				"window_handle": map[string]any{"type": "number", "description": "目标窗口句柄"},
				"client_area":   map[string]any{"type": "boolean", "description": "是否只截客户区"},
				"display_index": map[string]any{"type": "number", "description": "显示器索引"},
				"offset_x":      map[string]any{"type": "number", "description": "窗口内相对 X 偏移"},
				"offset_y":      map[string]any{"type": "number", "description": "窗口内相对 Y 偏移"},
				"width":         map[string]any{"type": "number", "description": "窗口内区域宽度"},
				"height":        map[string]any{"type": "number", "description": "窗口内区域高度"},
				"region_x":      map[string]any{"type": "number", "description": "绝对屏幕区域 X"},
				"region_y":      map[string]any{"type": "number", "description": "绝对屏幕区域 Y"},
				"region_w":      map[string]any{"type": "number", "description": "绝对屏幕区域宽度"},
				"region_h":      map[string]any{"type": "number", "description": "绝对屏幕区域高度"},
				"format":        map[string]any{"type": "string", "enum": []string{"png", "jpg"}, "description": "输出格式"},
				"note":          map[string]any{"type": "string", "description": "本次截图的一句话说明"},
			}, []string{}),
	}
}
