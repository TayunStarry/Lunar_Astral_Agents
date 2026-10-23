package lunarloop

import (
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"fmt"

	"github.com/dop251/goja"
)

// screenshotCapture 执行屏幕截图、压缩缩放并返回处理后的图片数据数组
// 参数: params 对象，字段对齐 module.CaptureRequest
//
//	mode, display_index, window_handle, window_title, client_area,
//	offset_x, offset_y, width, height,
//	region_x, region_y, region_w, region_h, format, quality, scale
//
// 返回值: [Array<Object>, error] 包含 base64/format/width/height 的结果对象数组和错误信息
// 注意：此函数内部已集成 ResizeImage 处理，返回的 base64 字段格式为 "data:image/[format];base64,[data]"
// 结果对象额外携带本次截图的实际元信息：mode / window_title / window_handle / source_width / source_height
func (class *Runtime) screenshotCapture(call goja.FunctionCall) goja.Value {
	req := module.CaptureRequest{Mode: module.ModeAuto, Format: "png"}

	if len(call.Arguments) >= 1 {
		if params, ok := call.Argument(0).Export().(map[string]any); ok {
			req = parseCaptureRequest(params)
		}
	}

	// 执行截图
	result, err := module.Capture(req)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 统一在 Go 层完成图片压缩缩放处理，确保 base64 格式正确
	resized, err := module.ResizeImage(result.Image)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("截图后处理失败: %v", err)})
	}
	// 回填截图元信息：智能体据此确认实际截到的窗口/模式，而不是只看图片猜
	for _, frame := range resized {
		frame["mode"] = string(result.Mode)
		frame["window_title"] = result.WindowTitle
		frame["window_handle"] = result.WindowHandle
		frame["source_width"] = result.Width
		frame["source_height"] = result.Height
	}

	return class.runtime.ToValue([]any{resized, nil})
}

// parseCaptureRequest 从 goja 导出的对象解析 CaptureRequest
func parseCaptureRequest(params map[string]any) module.CaptureRequest {
	req := module.CaptureRequest{Mode: module.ModeAuto, Format: "png"}

	if v, ok := params["mode"].(string); ok && v != "" {
		req.Mode = module.CaptureMode(v)
	}
	req.DisplayIndex = toInt(params["display_index"])
	req.WindowHandle = toUint64(params["window_handle"])
	if v, ok := params["window_title"].(string); ok {
		req.WindowTitle = v
	}
	req.ClientArea = toBool(params["client_area"])
	req.OffsetX = toInt(params["offset_x"])
	req.OffsetY = toInt(params["offset_y"])
	req.Width = toInt(params["width"])
	req.Height = toInt(params["height"])
	req.RegionX = toInt(params["region_x"])
	req.RegionY = toInt(params["region_y"])
	req.RegionW = toInt(params["region_w"])
	req.RegionH = toInt(params["region_h"])
	if v, ok := params["format"].(string); ok && v != "" {
		req.Format = v
	}
	req.Quality = toInt(params["quality"])
	if v, ok := params["scale"].(string); ok && v != "" {
		req.Scale = v
	}
	return req
}

// toInt 将 goja 导出的数值转换为 int（goja 可能返回 int64 或 float64）
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	default:
		return 0
	}
}

// toUint64 将 goja 导出的数值转换为 uint64（窗口句柄在 JS 侧以 number 传递）
func toUint64(v any) uint64 {
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return 0
		}
		return uint64(n)
	case int64:
		if n <= 0 {
			return 0
		}
		return uint64(n)
	case int:
		if n <= 0 {
			return 0
		}
		return uint64(n)
	case uint64:
		return n
	default:
		return 0
	}
}

// toBool 将 goja 导出的值转换为 bool
func toBool(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// screenshotGetDisplays 获取所有显示器信息
// 返回值: [Array<{index, x, y, width, height}>, error]
func (class *Runtime) screenshotGetDisplays(call goja.FunctionCall) goja.Value {
	displays := module.GetDisplays()

	// 转换为 []map[string]any 以便 goja 正确序列化
	result := make([]map[string]any, len(displays))
	for i, d := range displays {
		result[i] = map[string]any{
			"index":  d["index"],
			"x":      d["x"],
			"y":      d["y"],
			"width":  d["width"],
			"height": d["height"],
		}
	}

	return class.runtime.ToValue([]any{result, nil})
}

// screenshotListWindows 枚举可作为截图目标的可见顶层窗口
// 返回值: [Array<{handle, title, class, pid, process, x, y, width, height, client_w, client_h, minimized, foreground}>, error]
func (class *Runtime) screenshotListWindows(call goja.FunctionCall) goja.Value {
	windows, err := module.ListWindows()
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	result := make([]map[string]any, len(windows))
	for i, item := range windows {
		result[i] = map[string]any{
			"handle":     item.Handle,
			"title":      item.Title,
			"class":      item.Class,
			"pid":        item.ProcessID,
			"process":    item.Process,
			"x":          item.X,
			"y":          item.Y,
			"width":      item.Width,
			"height":     item.Height,
			"client_w":   item.ClientW,
			"client_h":   item.ClientH,
			"minimized":  item.Minimized,
			"foreground": item.Foreground,
		}
	}

	return class.runtime.ToValue([]any{result, nil})
}
