package adapters

import (
	"encoding/base64"
	"fmt"
	"screenshot"

	"github.com/dop251/goja"
)

// screenshotCapture 执行屏幕截图并返回 base64 编码的图片数据
// 参数: displayIndex, region, scale, format, quality
// 返回值: [string, error] base64 数据 URI 和错误信息
func (class *Runtime) screenshotCapture(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("screenshotCapture 参数不足，需要 displayIndex")})
	}

	req := screenshot.ScreenshotRequest{
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
	imgData, _, contentType, err := screenshot.Screenshot(req)
	if err != nil {
		return class.runtime.ToValue([]any{"", err})
	}

	// 编码为 base64 数据 URI
	base64Data := base64.StdEncoding.EncodeToString(imgData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	return class.runtime.ToValue([]any{dataURI, nil})
}

// screenshotGetDisplays 获取所有显示器信息
// 返回值: [Array<{index, x, y, width, height}>, error]
func (class *Runtime) screenshotGetDisplays(call goja.FunctionCall) goja.Value {
	displays := screenshot.GetDisplays()

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
