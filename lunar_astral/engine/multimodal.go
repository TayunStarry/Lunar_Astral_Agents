package engine

import (
	"LunarSubsystem/MultimodalAnalysis/module"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// videoMedia 将视频本地化并写入 llama-server 媒体目录，供感知者以 file:// 引用观看
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径
// 超过 60 秒的视频自动按 60 秒分段（FFmpeg 流复制切分）
// 返回值: [Array<{file: string, start: number, end: number}>, error] 媒体片段列表和错误信息
func (class *Runtime) videoMedia(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	inputFile, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("inputFile必须是字符串")})
	}

	segments, err := module.VideoToMedia(inputFile)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 转换为TypeScript可处理的格式
	result := make([]map[string]any, len(segments))
	for i, seg := range segments {
		result[i] = map[string]any{
			"file":  seg.File,
			"start": seg.Start,
			"end":   seg.End,
		}
	}

	return class.runtime.ToValue([]any{result, nil})
}

// audioWav 将音频本地化并转换为 16kHz 单声道 WAV，返回 base64 编码
// 输入支持 HTTP(S) URL、data:audio/xxx;base64 URI 与本地文件路径
// 返回值: [string, error] WAV 音频的 base64 编码和错误信息
func (class *Runtime) audioWav(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足")})
	}

	inputFile, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("inputFile必须是字符串")})
	}

	base64Data, err := module.AudioToWavBase64(inputFile)
	if err != nil {
		return class.runtime.ToValue([]any{"", err})
	}
	return class.runtime.ToValue([]any{base64Data, nil})
}

// isAnimatedImage 判断入参是否为动态图（多帧 GIF / APNG / 动态 WebP）
// 入参支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地文件路径）
// 返回值: [bool, error] 是否动态图与错误信息
func (class *Runtime) isAnimatedImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("参数不足")})
	}

	bytesData, err := resolveImageBytes(call.Argument(0))
	if err != nil {
		return class.runtime.ToValue([]any{false, err})
	}
	return class.runtime.ToValue([]any{module.IsAnimatedImage(bytesData), nil})
}

// animatedImageToVideo 将动态图编码为慢放视频并写入 llama-server 媒体目录
// 入参支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地文件路径）
// 返回值: [Array<{file: string, start: number, end: number}>, error] 媒体片段列表和错误信息
func (class *Runtime) animatedImageToVideo(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	bytesData, err := resolveImageBytes(call.Argument(0))
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	segments, err := module.AnimatedImageToMedia(bytesData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 转换为TypeScript可处理的格式
	result := make([]map[string]any, len(segments))
	for i, seg := range segments {
		result[i] = map[string]any{
			"file":  seg.File,
			"start": seg.Start,
			"end":   seg.End,
		}
	}
	return class.runtime.ToValue([]any{result, nil})
}

// resolveImageBytes 将 goja 入参解析为图片字节数据
// 支持字节数据（Blob/File/Uint8Array/字节字符串）与来源地址（HTTP(S) URL、data: URI、本地路径）
func resolveImageBytes(arg goja.Value) ([]byte, error) {
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil, fmt.Errorf("图片数据为空")
	}

	switch data := arg.Export().(type) {
	case []byte:
		return data, nil
	case string:
		// 来源地址：交给模块按 URL / data URI / 本地路径读取
		if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") || strings.HasPrefix(data, "data:") {
			return module.ImageSourceToBytes(data)
		}
		if path, ok := strings.CutPrefix(data, "file://"); ok {
			return module.ImageSourceToBytes(path)
		}
		// 其余字符串按原始字节处理（与 resizeImage 的既有约定一致）
		return []byte(data), nil
	case map[string]any:
		// goja 中的 Blob/File 会转换为携带 buffer / data 字段的对象
		if buffer, ok := data["buffer"].([]byte); ok {
			return buffer, nil
		}
		if raw, ok := data["data"].([]byte); ok {
			return raw, nil
		}
		return nil, fmt.Errorf("不支持的 Blob/File 数据格式")
	default:
		return nil, fmt.Errorf("不支持的图片数据类型")
	}
}

// resizeImage 适配TypeScript调用的标准图片处理：解码 → 格式校验/转码 → 等比例缩放到 1024 → 输出 base64
// 入参与 isAnimatedImage / animatedImageToVideo 一致：字节数据、HTTP(S) URL、data:image URI、本地路径
// 动态图在此按首帧静态降级；动态图理解请先用 isAnimatedImage 判定并走 animatedImageToVideo
// 返回值: [Array<Object>, error] 处理结果数组（恒为单元素）和错误信息
func (class *Runtime) resizeImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	// 统一入参解析：地址类入参由 Go 侧下载/解码，字节类入参直接使用
	bytesData, err := resolveImageBytes(call.Argument(0))
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 调用图片缩放函数，返回图片数据数组
	results, err := module.ResizeImage(bytesData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{results, nil})
}

// generateImage 适配TypeScript调用的图片生成功能，处理图片生成参数并返回结果
// 返回值: [Object, error] 图片生成结果和错误信息
func (class *Runtime) generateImage(call goja.FunctionCall) goja.Value {
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

	width := 768
	if w, ok := params["width"].(float64); ok {
		width = int(w)
	}

	height := 768
	if h, ok := params["height"].(float64); ok {
		height = int(h)
	}

	steps := 24
	if s, ok := params["steps"].(float64); ok {
		steps = int(s)
	}

	strength := 0.75
	if st, ok := params["strength"].(float64); ok {
		strength = st
	}

	cfgScale := 2.0
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
	result, err := module.GenerateImage(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg, false)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// atob 适配TypeScript调用的base64解码功能，处理base64编码的字符串并返回解码结果
// 返回值: string 解码后的字符串
func (class *Runtime) atob(call goja.FunctionCall) goja.Value {
	input := call.Argument(0).String()

	// Go 的标准 Base64 解码
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		panic(class.runtime.NewGoError(err))
	}
	return class.runtime.ToValue(string(decoded))
}

// hashBytes 适配TypeScript调用的内容哈希计算：对字节数据计算 SHA-256，截取前 16 位十六进制
// 与前端文件哈希命名约定一致（calculateFileHash：SHA-256 取前 16 位），供参考图等按内容哈希命名
// 入参解析与 resizeImage 一致：字节数据（Uint8Array 等）直接使用，地址类由 Go 侧下载/解码
// 返回值: [string, error] 哈希结果和错误信息
func (class *Runtime) hashBytes(call goja.FunctionCall) goja.Value {
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return class.runtime.ToValue([]any{"", fmt.Errorf("数据为空")})
	}
	bytesData, err := resolveImageBytes(arg)
	if err != nil {
		return class.runtime.ToValue([]any{"", err})
	}
	sum := sha256.Sum256(bytesData)
	return class.runtime.ToValue([]any{hex.EncodeToString(sum[:])[:16], nil})
}
