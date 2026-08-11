package adapters

import (
	"LunarSubsystem/image_processor/module"
	"fmt"

	"github.com/dop251/goja"
)

// screenshotCapture 执行屏幕截图、压缩缩放并返回处理后的图片数据数组
// 参数: displayIndex, region, scale, format, quality
// 返回值: [Array<Object>, error] 包含 base64/format/width/height 的结果对象数组和错误信息
// 注意：此函数内部已集成 ResizeImage 处理，返回的 base64 字段格式为 "data:image/[format];base64,[data]"
func (class *Runtime) screenshotCapture(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("screenshotCapture 参数不足，需要 displayIndex")})
	}

	req := module.ScreenshotRequest{
		DisplayIndex: -1, // 默认截取主显示器
		Format:       "png",
	}

	// 解析 displayIndex
	displayIndex := int(call.Argument(0).ToInteger())
	req.DisplayIndex = displayIndex

	// 解析可选参数 region
	if len(call.Arguments) >= 2 {
		region, ok := call.Argument(1).Export().(string)
		if ok && region != "" {
			req.Region = region
		}
	}

	// 解析可选参数 scale
	if len(call.Arguments) >= 3 {
		scale, ok := call.Argument(2).Export().(string)
		if ok && scale != "" {
			req.Scale = scale
		}
	}

	// 解析可选参数 format
	if len(call.Arguments) >= 4 {
		format, ok := call.Argument(3).Export().(string)
		if ok && format != "" {
			req.Format = format
		}
	}

	// 解析可选参数 quality
	if len(call.Arguments) >= 5 {
		req.Quality = int(call.Argument(4).ToInteger())
	}

	// 执行截图
	imgData, _, _, err := module.Screenshot(req)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 统一在 Go 层完成图片压缩缩放处理，确保 base64 格式正确
	result, err := module.ResizeImage(imgData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("截图后处理失败: %v", err)})
	}

	return class.runtime.ToValue([]any{result, nil})
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
