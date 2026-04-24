package adapters

import (
	"LunarCore/hierarchy/image"
	"LunarCore/hierarchy/image/generate"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// keyframe 适配TypeScript调用的视频关键帧提取功能，转换为TypeScript可处理的格式
// 返回值: [Array<{filePath: string, timestamp: number, frameNum: number, data: string}>, error] 关键帧列表和错误信息
func (class *Adapters) keyframe(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	inputFile, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("inputFile必须是字符串")})
	}

	cacheDir, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("cacheDir必须是字符串")})
	}

	keyFrames, err := image.VideoKeyframeExtraction(inputFile, cacheDir)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 转换为TypeScript可处理的格式
	result := make([]map[string]any, len(keyFrames))
	for i, frame := range keyFrames {
		result[i] = map[string]any{
			"filePath":  frame.FilePath,
			"timestamp": frame.Timestamp,
			"frameNum":  frame.FrameNum,
			"data":      base64.StdEncoding.EncodeToString(frame.Data),
		}
	}

	return class.runtime.ToValue([]any{result, nil})
}

// resizeImage 适配TypeScript调用的图片缩放功能，处理图片数据并返回缩放结果
// 返回值: [Object, error] 缩放结果和错误信息
func (class *Adapters) resizeImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	imgData := call.Argument(0).Export()
	// 处理不同类型的图片数据
	var bytesData []byte

	switch data := imgData.(type) {
	case string:
		// 处理base64编码的图片
		if strings.HasPrefix(data, "data:image/") {
			// 移除base64头部
			data = strings.Split(data, ",")[1]
			var err error
			bytesData, err = base64.StdEncoding.DecodeString(data)
			if err != nil {
				return class.runtime.ToValue([]any{nil, fmt.Errorf("解码base64图片失败: %v", err)})
			}
		} else {
			// 直接将字符串转换为字节数组
			bytesData = []byte(data)
		}
	case []byte:
		// 直接使用字节数组
		bytesData = data
	case map[string]any:
		// 处理goja中转换为map的Blob/File类型
		if buffer, ok := data["buffer"].([]byte); ok {
			bytesData = buffer
		} else if data, ok := data["data"].([]byte); ok {
			bytesData = data
		} else {
			return class.runtime.ToValue([]any{nil, fmt.Errorf("不支持的 Blob/File 数据格式")})
		}
	default:
		return class.runtime.ToValue([]any{nil, fmt.Errorf("不支持的图片数据类型")})
	}

	// 调用图片缩放函数
	result, err := image.ResizeImage(bytesData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// generateImage 适配TypeScript调用的图片生成功能，处理图片生成参数并返回结果
// 返回值: [Object, error] 图片生成结果和错误信息
func (class *Adapters) generateImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	params, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("params必须是对象")})
	}

	// 提取参数
	prompt, ok := params["prompt"].(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("提示词不能为空")})
	}

	negativePrompt := ""
	if np, ok := params["negativePrompt"].(string); ok {
		negativePrompt = np
	}

	batchSize := 1
	if bs, ok := params["batchSize"].(float64); ok {
		batchSize = int(bs)
	}

	width := 512
	if w, ok := params["width"].(float64); ok {
		width = int(w)
	}

	height := 512
	if h, ok := params["height"].(float64); ok {
		height = int(h)
	}

	steps := 20
	if s, ok := params["steps"].(float64); ok {
		steps = int(s)
	}

	strength := 0.7
	if st, ok := params["strength"].(float64); ok {
		strength = st
	}

	cfgScale := 7.5
	if cs, ok := params["cfgScale"].(float64); ok {
		cfgScale = cs
	}

	seed := int64(0)
	if sd, ok := params["seed"].(float64); ok {
		seed = int64(sd)
	}

	initImg := ""
	if ii, ok := params["initImg"].(string); ok {
		initImg = ii
	}

	// 调用图片生成函数
	result, err := generate.GenerateImage(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// atob 适配TypeScript调用的base64解码功能，处理base64编码的字符串并返回解码结果
// 返回值: string 解码后的字符串
func (class *Adapters) atob(call goja.FunctionCall) goja.Value {
	input := call.Argument(0).String()

	// Go 的标准 Base64 解码
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		panic(class.runtime.NewGoError(err))
	}
	return class.runtime.ToValue(string(decoded))
}
