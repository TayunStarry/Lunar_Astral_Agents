package adapters

import (
	"LunarSubsystem/ImageProcessor/module"
	"fmt"

	"github.com/dop251/goja"
)

// screenshotCapture 执行屏幕截图、压缩缩放并返回处理后的图片数据数组
// 参数: params 对象，字段对齐 module.CaptureRequest
//
//	mode, display_index, offset_x, offset_y, width, height,
//	region_x, region_y, region_w, region_h, format, quality, scale
//
// 返回值: [Array<Object>, error] 包含 base64/format/width/height 的结果对象数组和错误信息
// 注意：此函数内部已集成 ResizeImage 处理，返回的 base64 字段格式为 "data:image/[format];base64,[data]"
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

	return class.runtime.ToValue([]any{resized, nil})
}

// parseCaptureRequest 从 goja 导出的对象解析 CaptureRequest
func parseCaptureRequest(params map[string]any) module.CaptureRequest {
	req := module.CaptureRequest{Mode: module.ModeAuto, Format: "png"}

	if v, ok := params["mode"].(string); ok && v != "" {
		req.Mode = module.CaptureMode(v)
	}
	req.DisplayIndex = toInt(params["display_index"])
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
