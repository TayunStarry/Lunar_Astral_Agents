package StarLTP

// ==== 媒体能力实现：视频抽帧 video.frames / 图像缩放与编码 image.resize|convert / WebLTP 网络搜索 ====
// 复用 MultimodalAnalysis 模块的抽帧/图像基础组元与 WebLTP 进程内搜索流水线，
// 沙箱侧契约见 docs/LTP 9.1 Flash.d.ts。

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	multimodal "LunarSubsystem/MultimodalAnalysis/module"
	webltp "CrystalAstral/agent/WebLTP"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// ==== 视频抽帧 video.frames（allow-file；URL 源运行时另需 allow-network） ====

// maxVideoFrames 单次抽帧最大输出帧数：均衡脚本便利与内存/耗时，超限时等距抽样。
const maxVideoFrames = 60

// frameDedupThreshold 相邻帧相似度阈值：差异 <= 该值视为冗余跳过（默认开启去重）。
const frameDedupThreshold = 0.45

// engineVideoFrames 按采样计划从视频抽取多帧，返回 { success, frames:[...], count }。
// source 支持相对插件数据目录路径 / http(s) URL / data:video;base64 URI。
func engineVideoFrames(p *plugin, source string, opts map[string]any) map[string]any {
	source = strings.TrimSpace(source)
	if source == "" {
		return map[string]any{"success": false, "error": "缺少视频来源（相对数据目录路径 / http(s) URL / data:video URI）"}
	}
	localPath, cleanup, err := videoSourceToLocal(p, source)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if cleanup != nil {
		defer cleanup()
	}

	duration, derr := multimodal.GetVideoDuration(localPath)
	if derr != nil {
		return map[string]any{"success": false, "error": "探测视频时长失败: " + derr.Error()}
	}

	sampling := videoSamplingPlan(duration, opts)

	// 编解码参数（all 走 opts，显式容错）
	maxDim := optsInt(opts, "max_dim", 640)
	format := mediaFormat(optsFormat(opts), "jpeg")
	quality := optsInt(opts, "quality", 85)
	if quality < 1 {
		quality = 1
	} else if quality > 100 {
		quality = 100
	}
	dedup := true
	if d, ok := opts["dedup"].(bool); ok {
		dedup = d
	}

	frames := make([]map[string]any, 0, len(sampling))
	var prev image.Image
	skipped := 0
	for i, t := range sampling {
		img, ferr := extractVideoFrameAt(localPath, t, maxDim)
		if ferr != nil {
			LoggerGeneral.Warn(ServiceName, "[video.frames] 第%d帧(%ss)抽取失败，跳过: %v", i, formatSeconds(t), ferr)
			skipped++
			continue
		}
		if dedup && prev != nil && multimodal.CalculateImageDifference(prev, img) <= frameDedupThreshold {
			skipped++
			continue
		}
		prev = img
		enc, ct, eerr := encodeImageToFormat(img, format, quality)
		if eerr != nil {
			LoggerGeneral.Warn(ServiceName, "[video.frames] 第%d帧编码失败，跳过: %v", i, eerr)
			skipped++
			continue
		}
		frames = append(frames, map[string]any{
			"data":      "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(enc),
			"timestamp": formatSeconds(t),
			"width":     img.Bounds().Dx(),
			"height":    img.Bounds().Dy(),
			"format":    strings.TrimPrefix(ct, "image/"),
			"index":     i,
		})
	}
	if len(frames) == 0 {
		return map[string]any{"success": false, "error": "未抽取到可用帧（视频文件可能损坏或不可解码）"}
	}
	LoggerGeneral.Info(ServiceName, "LTP9 video.frames: 计划=%d 帧, 输出=%d 帧(去重/失败跳过=%d), 来源=%s", len(sampling), len(frames), skipped, source)
	res := map[string]any{"success": true, "frames": frames, "count": len(frames)}
	if skipped > 0 {
		res["skipped"] = skipped
	}
	return res
}

// videoSourceToLocal 把沙箱侧视频来源解析为本地可读路径；URL / dataURI 落到临时文件并返回清理函数。
func videoSourceToLocal(p *plugin, source string) (string, func(), error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		if !p.hasPerm("allow-network") {
			return "", nil, fmt.Errorf("video.frames 的 URL 视频源需要 allow-network 权限")
		}
		tmp, err := downloadVideoToTemp(source)
		if err != nil {
			return "", nil, err
		}
		return tmp, func() { os.Remove(tmp) }, nil
	case strings.HasPrefix(source, "data:"):
		tmp, err := dataVideoToTemp(source)
		if err != nil {
			return "", nil, err
		}
		return tmp, func() { os.Remove(tmp) }, nil
	default:
		real, err := scopedPath(p, source)
		if err != nil {
			return "", nil, err
		}
		if _, serr := os.Stat(real); serr != nil {
			return "", nil, fmt.Errorf("视频文件不存在: %s", source)
		}
		return real, nil, nil
	}
}

// videoSamplingPlan 计算抽帧时间点列表（秒）：times 显式 > count 等距 > fps 均匀网格。
func videoSamplingPlan(duration float64, opts map[string]any) []float64 {
	if times, ok := opts["times"].([]any); ok && len(times) > 0 {
		ts := make([]float64, 0, len(times))
		for _, v := range times {
			if f, ok := numToFloat(v); ok && f >= 0 {
				ts = append(ts, clampf(f, 0, duration))
			}
		}
		if len(ts) > 0 {
			return ts
		}
	}
	if c := optsInt(opts, "count", 0); c > 0 {
		return equidistantTimes(c, duration)
	}
	fps := optsFloat(opts, "fps", 5.0)
	if fps <= 0 {
		fps = 5.0
	}
	n := int(duration*fps) + 1
	if n > maxVideoFrames {
		return equidistantTimes(maxVideoFrames, duration)
	}
	ts := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		ts = append(ts, float64(i)/fps)
	}
	return ts
}

// equidistantTimes 生成 n 个等距时间点（恒含时间 0；n=1 时仅返回 0）。
func equidistantTimes(n int, duration float64) []float64 {
	if n < 1 {
		n = 1
	}
	if n > maxVideoFrames {
		n = maxVideoFrames
	}
	if duration <= 0 || n == 1 {
		return []float64{0}
	}
	ts := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		ts = append(ts, duration*float64(i)/float64(n-1))
	}
	return ts
}

// extractVideoFrameAt 用 FFmpeg 抽取 t 秒处单帧并缩放至 maxDim（0 表示不缩放），返回 image。
func extractVideoFrameAt(localPath string, t float64, maxDim int) (image.Image, error) {
	buf := &bytes.Buffer{}
	stream := ffmpeg.Input(localPath, ffmpeg.KwArgs{"ss": strconv.FormatFloat(t, 'f', 3, 64)}).
		Output("pipe:1", ffmpeg.KwArgs{
			"frames:v": "1",
			"f":        "image2pipe",
			"vcodec":   "mjpeg",
			"qscale:v": "2",
		})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	if err := stream.WithOutput(buf, os.Stderr).Run(); err != nil {
		return nil, fmt.Errorf("FFmpeg 抽取 %s 处画面失败: %v", formatSeconds(t), err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg 在 %s 处无画面输出", formatSeconds(t))
	}
	img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("解码 %s 处帧失败: %v", formatSeconds(t), err)
	}
	rgba := multimodal.ToRGBA(img)
	if maxDim > 0 {
		rgba = multimodal.ResizeToFit(rgba, maxDim, maxDim)
	}
	return rgba, nil
}

// downloadVideoToTemp 下载 HTTP 视频到临时文件。
func downloadVideoToTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("下载视频失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载视频失败: HTTP %d", resp.StatusCode)
	}
	ext := "mp4"
	if pathPart := strings.SplitN(url, "?", 2)[0]; pathPart != "" {
		if i := strings.LastIndex(pathPart, "."); i >= 0 && i < len(pathPart)-1 {
			cand := strings.ToLower(pathPart[i:])
			if multimodal.IsSupportedVideoFormat("v" + cand) {
				ext = strings.TrimPrefix(cand, ".")
			}
		}
	}
	tmp, err := os.CreateTemp("", "ltp9_video_*."+ext)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("写入临时视频失败: %v", err)
	}
	return tmp.Name(), nil
}

// dataVideoToTemp 把 data:video/...;base64 URI 解码为临时文件。
func dataVideoToTemp(uri string) (string, error) {
	parts := strings.SplitN(uri, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
		return "", fmt.Errorf("无效的 data URI 格式")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %v", err)
	}
	tmp, err := os.CreateTemp("", "ltp9_video_data_*.mp4")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	if _, werr := tmp.Write(data); werr != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("写入临时视频失败: %v", werr)
	}
	tmp.Close()
	return tmp.Name(), nil
}

// ==== 图像缩放与编码 image.resize / image.convert（allow-file） ====

// imageBytesFromSource 读取插件数据目录图片字节（path 为 data:base64 URI 时内联解码）。
func imageBytesFromSource(p *plugin, path string) ([]byte, error) {
	if strings.HasPrefix(path, "data:") {
		comma := strings.Index(path, ",")
		if comma < 0 {
			return nil, fmt.Errorf("无效的 data URI")
		}
		return base64.StdEncoding.DecodeString(path[comma+1:])
	}
	real, err := scopedPath(p, path)
	if err != nil {
		return nil, err
	}
	b, rerr := os.ReadFile(real)
	if rerr != nil {
		return nil, fmt.Errorf("读取图片失败: %v", rerr)
	}
	if !isImageContent(b) {
		return nil, fmt.Errorf("非有效图片: %s", path)
	}
	return b, nil
}

// engineImageResize 缩放（max_dim 等比 / width+height 精确）并按 format 重新编码，返回 data URI。
func engineImageResize(p *plugin, path string, opts map[string]any) (any, error) {
	src, err := imageBytesFromSource(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	img, srcFormat, derr := image.Decode(bytes.NewReader(src))
	if derr != nil {
		return rwResult{Success: false, Error: "解码图片失败: " + derr.Error()}, nil
	}
	rgba := multimodal.ToRGBA(img)

	format := mediaFormat(optsFormat(opts), defaultOutputFormat(srcFormat))
	quality := optsInt(opts, "quality", 90)
	if quality < 1 {
		quality = 1
	} else if quality > 100 {
		quality = 100
	}
	maxDim := optsInt(opts, "max_dim", 0)
	width := optsInt(opts, "width", 0)
	height := optsInt(opts, "height", 0)

	var out image.Image
	switch {
	case maxDim > 0:
		out = multimodal.ResizeToFit(rgba, maxDim, maxDim)
	case width > 0 && height > 0:
		out = imaging.Resize(rgba, width, height, imaging.Lanczos)
	default:
		out = rgba
	}

	enc, ct, eerr := encodeImageToFormat(out, format, quality)
	if eerr != nil {
		return rwResult{Success: false, Error: eerr.Error()}, nil
	}
	b := out.Bounds()
	return imageResult(enc, ct, b.Dx(), b.Dy()), nil
}

// engineImageConvert 仅重编码图片为指定格式（编码格式化），不改变尺寸。
func engineImageConvert(p *plugin, path, format string, opts map[string]any) (any, error) {
	format = mediaFormat(format, "jpeg")
	src, err := imageBytesFromSource(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	img, _, derr := image.Decode(bytes.NewReader(src))
	if derr != nil {
		return rwResult{Success: false, Error: "解码图片失败: " + derr.Error()}, nil
	}
	quality := optsInt(opts, "quality", 90)
	if quality < 1 {
		quality = 1
	} else if quality > 100 {
		quality = 100
	}
	enc, ct, eerr := encodeImageToFormat(img, format, quality)
	if eerr != nil {
		return rwResult{Success: false, Error: eerr.Error()}, nil
	}
	b := img.Bounds()
	return imageResult(enc, ct, b.Dx(), b.Dy()), nil
}

// ==== Web-LTP 网络搜索 agent.search（双权限：allow-agent 绑定 + allow-network 执行） ====

// engineAgentSearch 调用进程内 Web-LTP 搜索流水线，返回自然语言搜索报告。
func engineAgentSearch(p *plugin, instruction string) map[string]any {
	if !p.hasPerm("allow-network") {
		return map[string]any{"success": false, "error": "agent.search 需要 allow-network 权限"}
	}
	text, err := webltp.Run(instruction)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if strings.TrimSpace(text) == "" {
		text = "搜索已完成"
	}
	return map[string]any{"success": true, "text": text}
}

// ==== 共享工具 ====

// imageResult 构造图片类沙箱 API 的统一结果（text 为 data URI）。
func imageResult(enc []byte, contentType string, w, h int) map[string]any {
	return map[string]any{
		"success": true,
		"text":    "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(enc),
		"width":   w,
		"height":  h,
		"format":  strings.TrimPrefix(contentType, "image/"),
	}
}

// encodeImageToFormat 把 image 编码为 jpeg/png/webp，返回字节与 content type。
func encodeImageToFormat(img image.Image, format string, quality int) ([]byte, string, error) {
	buf := &bytes.Buffer{}
	switch format {
	case "jpg", "jpeg":
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", fmt.Errorf("JPEG 编码失败: %v", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	case "png":
		if err := png.Encode(buf, img); err != nil {
			return nil, "", fmt.Errorf("PNG 编码失败: %v", err)
		}
		return buf.Bytes(), "image/png", nil
	case "webp":
		if err := webp.Encode(buf, img, &webp.Options{Quality: float32(quality)}); err != nil {
			return nil, "", fmt.Errorf("WebP 编码失败: %v", err)
		}
		return buf.Bytes(), "image/webp", nil
	}
	return nil, "", fmt.Errorf("不支持的编码格式: %s", format)
}

// optsFormat 读取 opts.format 字符串（规范化为小写，去空白）。
func optsFormat(opts map[string]any) string {
	if opts == nil {
		return ""
	}
	if s, ok := opts["format"].(string); ok {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

// mediaFormat 规范化目标格式；空/非法时回落默认值。
func mediaFormat(format, fallback string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "jpeg", "jpg", "png", "webp":
		if format == "jpg" {
			return "jpeg"
		}
		return format
	}
	return fallback
}

// optsInt 读取 opts[key] 数值（goja 整型为 int64、浮点为 float64，兼容两者），缺失/非法回落默认。
func optsInt(opts map[string]any, key string, def int) int {
	if opts == nil {
		return def
	}
	switch v := opts[key].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// optsFloat 读取 opts[key] 浮点值（兼容 int64/float64），缺失/非法回落默认。
func optsFloat(opts map[string]any, key string, def float64) float64 {
	if opts == nil {
		return def
	}
	switch v := opts[key].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	}
	return def
}

// numToFloat 把 goja 数组元素（含 int64/float64）转为 float64。
func numToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

// defaultOutputFormat 未指定格式时按源格式保留（PNG 保透明），其余统一 JPEG。
func defaultOutputFormat(srcFormat string) string {
	if strings.EqualFold(srcFormat, "png") {
		return "png"
	}
	return "jpeg"
}

// clampf 把数值钳制到 [lo, hi]。
func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// formatSeconds 秒 → "HH:MM:SS" 时间刻度字符串。
func formatSeconds(t float64) string {
	d := time.Duration(t * float64(time.Second))
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}