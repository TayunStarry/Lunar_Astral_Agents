package lunarloop

import (
	module "CorePersonality/engine/multimodal"
	"LunarSubsystem/GeneralConfig"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

// videoMedia 将视频本地化并写入 llama-server 媒体目录，供需要 file:// 引用的调用方使用
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

// videoFrames 将视频抽帧为图片序列（序列帧解读模式），供不支持 file:// 的架构使用
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径
// 分组与 file 模式一致（每 60 秒一组），组内按 0.5fps 采样并编码为 JPEG data URI
// 返回值: [Array<{start, end, frames: Array<{image, time}>}>, error] 分组采样帧列表和错误信息
func (class *Runtime) videoFrames(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	inputFile, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("inputFile必须是字符串")})
	}

	groups, err := module.VideoToFrames(inputFile)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	return class.runtime.ToValue([]any{frameGroupsToValue(groups), nil})
}

// frameGroupsToValue 把抽帧分组转换为 TypeScript 可处理的对象数组
func frameGroupsToValue(groups []module.MediaFrameGroup) []map[string]any {
	result := make([]map[string]any, len(groups))
	for i, g := range groups {
		frames := make([]map[string]any, len(g.Frames))
		for j, f := range g.Frames {
			frames[j] = map[string]any{"image": f.Image, "time": f.Time}
		}
		result[i] = map[string]any{"start": g.Start, "end": g.End, "frames": frames}
	}
	return result
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

// animatedImageToFrames 将动态图直接解码为图片帧序列（长边受限、至多 64 帧）
// 入参支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地文件路径）
// 返回值: [Array<{start, end, frames: Array<{image, time}>}>, error] 分组帧列表和错误信息
func (class *Runtime) animatedImageToFrames(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	bytesData, err := resolveImageBytes(call.Argument(0))
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	groups, err := module.AnimatedImageToFrames(bytesData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	return class.runtime.ToValue([]any{frameGroupsToValue(groups), nil})
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
		// 本机服务路径（/file/read/... 或 /file/miniature/...）：转换为 LocalDir 相对路径读取。
		// 句柄唯一性归一后，绘制者的 lastResultPath 可能已是服务 URL。
		for _, prefix := range []string{"/file/read/", "/file/miniature/"} {
			if rel, ok := strings.CutPrefix(data, prefix); ok {
				// 视觉读取 URL 自带 ?tag=... 查询参数：查询串属于 HTTP 语义，不是文件路径。
				// 不剥离会按「带问号的文件名」查找而必然失败，动态图判定随之报错并退化为静态图。
				if cut := strings.IndexAny(rel, "?#"); cut >= 0 {
					rel = rel[:cut]
				}
				if decoded, err := url.PathUnescape(rel); err == nil {
					rel = decoded
				}
				if abs, found := resolveLocalFilePath(rel); found {
					return module.ImageSourceToBytes(abs)
				}
				return nil, fmt.Errorf("服务路径引用的文件不存在: %s", data)
			}
		}
		// 来源地址：交给模块按 URL / data URI / 本地路径读取
		if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") || strings.HasPrefix(data, "data:") {
			return module.ImageSourceToBytes(data)
		}
		// file:// 明确表示「这是一个文件引用」，必须按路径解析：
		// 绝对路径直接读，相对路径按 LocalDir 解析（资源目录里的 blob_path 就是相对形式）。
		// 读不到时报错而不是回退成裸字节 —— 否则调用方只能在几千字节的乱码里猜原因。
		if path, ok := strings.CutPrefix(data, "file://"); ok {
			if abs, found := resolveLocalFilePath(path); found {
				return module.ImageSourceToBytes(abs)
			}
			return nil, fmt.Errorf("file:// 引用的文件不存在: %s", path)
		}
		// 资源目录给出的相对路径（如 multimedia/<sha16>.jpg）：存在则按文件读
		if abs, found := resolveLocalFilePath(data); found {
			return module.ImageSourceToBytes(abs)
		}
		// 其余字符串按原始字节处理（仅供 Go 侧管线解码，不得回流到 image_url）
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

// resolveLocalFilePath 把路径解析为可读的本地文件绝对路径
//
// 解析顺序：绝对路径原样 → 相对 LocalDir 的路径。返回 false 表示两处都没有这个文件
// （调用方据此决定是报错还是回退成裸字节）。
func resolveLocalFilePath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	candidate := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(candidate) {
		if fileExists(candidate) {
			return candidate, true
		}
		return "", false
	}
	localRel := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), candidate)
	if fileExists(localRel) {
		return localRel, true
	}
	// 兼容以进程工作目录为基准的相对路径
	if fileExists(candidate) {
		return candidate, true
	}
	return "", false
}

// fileExists 判断路径是否为存在的普通文件
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// 注意：这里**不提供** resizeImage。图片缩放与尺寸约束只允许由视觉读取管线
// （/file/miniature/ 端点与 visionRead → VisionCopy）完成；思维链拿不到
// 任何"自己把原图缩一下"的入口。历史实现见 docs/adr/0008「思维链图片来源唯一化」。

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
	if bs := asInt(params["batchSize"]); bs > 0 {
		batchSize = bs
	}

	// 尺寸与步数缺省交给扩散模块归一化：步数优先取当前方案推荐步数，
	// 方案未配置 steps 时才回退 sd_steps；尺寸回退 sd_image_size。
	// 传 0 表示"未指定"，避免在此处写死一套与配置不一致的默认值。
	// 注意：goja 导出的数值类型可能是 float64 / int64 / int，必须逐类型接受 ——
	// 只认 float64 会让边长静默变 0，引擎随即退回默认方形，参考图比例被悄悄丢弃。
	width := asInt(params["width"])
	height := asInt(params["height"])
	if os.Getenv("LUNAR_DEBUG_GENPARAMS") != "" {
		fmt.Printf("[调试] generateImage 入参类型: width=%T(%v) height=%T(%v)\n",
			params["width"], params["width"], params["height"], params["height"])
	}

	steps := asInt(params["steps"])

	strength := 0.75
	if st := asFloat(params["strength"]); st > 0 {
		strength = st
	}

	cfgScale := 2.0
	if cs := asFloat(params["cfgScale"]); cs > 0 {
		cfgScale = cs
	}

	seed := int64(asInt(params["seed"]))

	initImg := ""
	if ii, ok := params["initImg"].(string); ok {
		initImg = ii
	}

	// 超分辨率重建：true 时生成完成后追加 RealESRGAN 放大（透传至扩散模块）
	superResolution := false
	if sr, ok := params["superResolution"].(bool); ok {
		superResolution = sr
	}

	// 调用图片生成函数
	result, err := module.GenerateImage(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg, superResolution)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// setDiffusionScheme 切换绘图模型方案（turbo / quality）。
//
// 状态保存在 Go 层，与图像工坊前端的 /generate/scheme 读写同一份记录；
// 只影响此后创建的生成任务，已入队任务按入队时的方案快照执行。
// 返回值: [string, error] 切换后的方案键名和错误信息
func (class *Runtime) setDiffusionScheme(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足")})
	}
	name, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("方案名必须是字符串")})
	}
	if err := GeneralConfig.SetActiveDiffusionScheme(name); err != nil {
		return class.runtime.ToValue([]any{"", err})
	}
	return class.runtime.ToValue([]any{GeneralConfig.CurrentDiffusionSchemeName(), nil})
}

// getDiffusionScheme 查询当前绘图模型方案
// 返回值: [string, error] 当前方案键名和错误信息
func (class *Runtime) getDiffusionScheme(call goja.FunctionCall) goja.Value {
	return class.runtime.ToValue([]any{GeneralConfig.CurrentDiffusionSchemeName(), nil})
}

// asInt 把 goja 导出的数值转换为 int（不支持的形态返回 0）
//
// 为什么不能直接断言 float64：goja 的 Export 对不同类型的 JS 数值可能给出
// float64 / int64 / int。历史实现只认 float64，一旦实际是 int64 就静默取 0 ——
// 调用方传的 1024x1536 会变成"未指定"，引擎退回默认方形，参考图比例被悄悄丢弃。
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case int32:
		return int(n)
	}
	return 0
}

// asFloat 把 goja 导出的数值转换为 float64（不支持的形态返回 0）
func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	}
	return 0
}

// upscaleImage 适配TypeScript调用的图像超分辨率重建：对已有图片执行 RealESRGAN 放大（4x）
// 入参支持字节数据（Uint8Array 等）与地址类（HTTP(S) URL、data:image URI、本地路径），
// 地址类由 Go 侧下载/解码后走 sd-cli -M upscale 管线；产物只回给 Go 侧管线，不回思维链。
// 返回值: [Object, error] 超分结果（base64/尺寸）和错误信息
func (class *Runtime) upscaleImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	// 统一入参解析：地址类入参由 Go 侧下载/解码，字节类入参直接使用
	bytesData, err := resolveImageBytes(call.Argument(0))
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 调用超分函数
	result, err := module.UpscaleImage(bytesData)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{result, nil})
}

// atob 适配TypeScript调用的base64解码功能，处理base64编码的字符串并返回解码结果
// 返回值: string 解码后的字符串
//
// 注意：返回的是 Go 字符串，goja 会按 UTF-8 转成 JS 字符串；二进制数据中的
// 非法 UTF-8 字节会被替换，因此**图片/音频等二进制内容不要用 atob**，
// 应改用 base64Decode 取字节数组。
func (class *Runtime) atob(call goja.FunctionCall) goja.Value {
	input := call.Argument(0).String()

	// Go 的标准 Base64 解码
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		panic(class.runtime.NewGoError(err))
	}
	return class.runtime.ToValue(string(decoded))
}

// base64Decode 适配 TypeScript 的二进制 base64 解码：返回字节数组，而不是字符串。
//
// 背景：atob 返回 Go string，goja 按 UTF-8 转 JS 字符串时会把 PNG/JPEG 中的
// 非法 UTF-8 字节替换成 U+FFFD；调用方再用 charCodeAt 还原就会得到 0xFD，
// 落盘后图片损坏。二进制内容必须走这个入口。
// 返回值: [ArrayBuffer, error]
func (class *Runtime) base64Decode(call goja.FunctionCall) goja.Value {
	input := strings.TrimSpace(call.Argument(0).String())
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{class.runtime.NewArrayBuffer(decoded), nil})
}

// hashBytes 适配TypeScript调用的内容哈希计算：对字节数据计算 SHA-256，截取前 16 位十六进制
// 与前端文件哈希命名约定一致（calculateFileHash：SHA-256 取前 16 位），供参考图等按内容哈希命名
// 入参解析与 upscaleImage 一致：字节数据（Uint8Array 等）直接使用，地址类由 Go 侧下载/解码
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
