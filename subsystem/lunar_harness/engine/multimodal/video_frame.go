package multimodal

// 视频/动态图抽帧组元与「图片序列帧」解读链路
//
// 本文件承载两类内容：
//  1. 抽帧基础组元（ExtractVideoFrameAt / EquidistantTimes / MaxVideoFrames / FormatSeconds）
//     —— 上提自 LTP9 引擎的 video.frames 实现（crystal_astral/engine/9.1-Flash/api_media.go），
//     作为全项目唯一的抽帧实现，供插件 API 与月华感知者共用，避免重复算法。
//  2. 序列帧解读链路（VideoToFrames / AnimatedImageToFrames）
//     —— 供不支持 llama-server 「媒体目录 + file:// 引用」机制的架构（如云端 OpenAI 兼容 API）使用：
//     本地抽帧为 base64 图片序列，以多个 image_url 提交，无需服务端解码视频文件。
//     采样网格与分组刻意与 file 模式对齐（1fps / 80s 一段），保证两种模式的视觉 token 量级一致。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MaxVideoFrames 单次抽帧的最大输出帧数上限（等距抽样时的硬上限）。
const MaxVideoFrames = 60

// MediaFrame 序列帧模式的单帧信息
type MediaFrame struct {
	// Image 帧图片的完整 data URI（data:image/jpeg;base64,...），可直接作为 image_url 提交
	Image string `json:"image"`
	// Time 帧在源视频中的时间点（秒）
	Time float64 `json:"time"`
}

// MediaFrameGroup 序列帧模式的时间分组，与 file 模式的 MediaSegment 同构
type MediaFrameGroup struct {
	// Start 分组起始时间（秒）
	Start float64 `json:"start"`
	// End 分组结束时间（秒）
	End float64 `json:"end"`
	// Frames 该时间分组内的采样帧（按时间顺序）
	Frames []MediaFrame `json:"frames"`
}

// ExtractVideoFrameAt 用 FFmpeg 抽取 t 秒处单帧并缩放至 maxDim（0 表示不缩放），返回 RGBA 图像
func ExtractVideoFrameAt(localPath string, t float64, maxDim int) (*image.RGBA, error) {
	buf := &bytes.Buffer{}
	stream := ffmpeg.Input(localPath, ffmpeg.KwArgs{"ss": strconv.FormatFloat(t, 'f', 3, 64)}).
		Output("pipe:1", ffmpeg.KwArgs{
			"frames:v": "1",
			"f":        "image2pipe",
			"vcodec":   "mjpeg",
			"qscale:v": "2",
		})
	if *config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*config.FfmpegPath)
	}
	if err := stream.WithOutput(buf, os.Stderr).Run(); err != nil {
		return nil, fmt.Errorf("FFmpeg 抽取 %s 处画面失败: %v", FormatSeconds(t), err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg 在 %s 处无画面输出", FormatSeconds(t))
	}
	img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("解码 %s 处帧失败: %v", FormatSeconds(t), err)
	}
	rgba := ToRGBA(img)
	if maxDim > 0 {
		rgba = ResizeToFit(rgba, maxDim, maxDim)
	}
	return rgba, nil
}

// EquidistantTimes 生成 n 个等距时间点（恒含时间 0；n=1 时仅返回 0），硬上限 MaxVideoFrames
func EquidistantTimes(n int, duration float64) []float64 {
	if n < 1 {
		n = 1
	}
	if n > MaxVideoFrames {
		n = MaxVideoFrames
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

// FormatSeconds 秒 → "HH:MM:SS" 时间刻度字符串
func FormatSeconds(t float64) string {
	d := time.Duration(t * float64(time.Second))
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// frameSampleTimes 生成分组 [start, end) 内的采样时间点：
// 以 AnimatedSampleFps（1fps，与 llama-server 的 --video-fps 一致）为网格步长，
// 与 file 模式由服务端抽帧的时间点分布保持同构（60 秒分组 = 60 帧）。
func frameSampleTimes(start float64, end float64) []float64 {
	span := end - start
	if span <= 0 {
		return []float64{start}
	}
	n := max(int(math.Ceil(span*AnimatedSampleFps)), 1)
	step := 1.0 / AnimatedSampleFps
	times := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		times = append(times, start+float64(i)*step)
	}
	return times
}

// extractFrames 按时间点抽帧并编码为 JPEG data URI；个别帧抽取/编码失败仅跳过，不中断整体抽取
func extractFrames(localPath string, times []float64, quality int) []MediaFrame {
	frames := make([]MediaFrame, 0, len(times))
	for i, t := range times {
		img, err := ExtractVideoFrameAt(localPath, t, MaxMediaWidth)
		if err != nil {
			logger.Warn("MultimodalAnalysis", "序列帧第%d帧(%s)抽取失败，跳过: %v", i, FormatSeconds(t), err)
			continue
		}
		buf := &bytes.Buffer{}
		if err := encodeImage(buf, img, "jpeg", quality); err != nil {
			logger.Warn("MultimodalAnalysis", "序列帧第%d帧(%s)编码失败，跳过: %v", i, FormatSeconds(t), err)
			continue
		}
		frames = append(frames, MediaFrame{
			Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
			Time:  t,
		})
	}
	return frames
}

// VideoToFrames 视频 → 按时间分组抽帧为 base64 图片序列（序列帧解读模式）
//
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
// 分组与 file 模式的 MediaSegment 一致（每 MediaChunkSeconds 秒一组），
// 组内按 AnimatedSampleFps 采样网格抽帧，长边不超过 MaxMediaWidth，JPEG 编码为 data URI。
// 不写媒体目录（无需服务端解码视频），全部产物均在内存中返回。
func VideoToFrames(inputFile string) ([]MediaFrameGroup, error) {
	localPath, cleanup, err := localizeVideoSource(inputFile)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	duration, err := GetVideoDuration(localPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	if duration > 3600 {
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	quality := *config.JPEGQuality
	segments := max(int((duration-0.001)/MediaChunkSeconds)+1, 1)
	groups := make([]MediaFrameGroup, 0, segments)
	for i := range segments {
		start := float64(i) * MediaChunkSeconds
		end := start + MediaChunkSeconds
		if end > duration {
			end = duration
		}
		frames := extractFrames(localPath, frameSampleTimes(start, end), quality)
		if len(frames) == 0 {
			return nil, fmt.Errorf("视频片段%d未抽取到可用帧", i)
		}
		groups = append(groups, MediaFrameGroup{Start: start, End: end, Frames: frames})
	}

	frameCount := 0
	for _, g := range groups {
		frameCount += len(g.Frames)
	}
	logger.Info("MultimodalAnalysis", "VideoToFrames: 时长 %.2fs → %d 组 / %d 帧, 来源=%s",
		duration, len(groups), frameCount, inputFile)
	return groups, nil
}

// AnimatedImageToFrames 动态图 → 智能筛选后的图片帧序列（单分组，<= 64 帧）
//
// 不再把动态图转码为慢放视频，也不盲目均匀抽帧：
// 先经分析通道做循环裁剪、运动窗口识别与近重复去重（animated_smart.go），
// 再通过 ffmpeg select 只解码选中的帧（长边约束到 AnimatedFrameMaxDim）。
func AnimatedImageToFrames(imgData []byte) ([]MediaFrameGroup, error) {
	if len(imgData) == 0 {
		return nil, fmt.Errorf("动态图数据为空")
	}
	if len(imgData) > maxAnimatedBytes {
		return nil, fmt.Errorf("动态图数据过大，超过50MB限制")
	}
	if !IsAnimatedImage(imgData) {
		return nil, fmt.Errorf("不是动态图（多帧 GIF / APNG / 动态 WebP）")
	}

	// 落到临时文件：ffmpeg 需要按扩展名/探测识别解复用器，管道输入对动态 WebP 不可靠
	format := detectImageFormat(imgData)
	tempFile, err := writeTempImage(imgData, format)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile)

	probe, probeErr := probeAnimatedInfo(tempFile)
	if probeErr != nil {
		logger.Warn("MultimodalAnalysis", "动态图帧数探测失败(%v)，回退为解码全部帧", probeErr)
	}
	totalFrames := probe.Frames
	if totalFrames <= 0 {
		totalFrames = animatedAnalyzeCap // 安全上限；极端长动画会被截断，但不会无限占用磁盘
	}

	// 分析通道：全部帧缩略为 32×32 灰度网格，派生帧差与感知哈希，
	// 支撑循环裁剪、运动窗口加密与近重复去重；失败时退回均匀骨架选帧。
	analysis, analyzeErr := analyzeAnimatedFrames(tempFile, totalFrames)
	if analyzeErr != nil {
		logger.Warn("MultimodalAnalysis", "动态图逐帧分析失败(%v)，回退为均匀骨架选帧", analyzeErr)
	}
	useSmart := analyzeErr == nil && len(analysis.Grids) > 0
	strategy := "均匀骨架"
	var indices []int
	framesTotal := probe.Frames
	if useSmart {
		indices = SelectSmartAnimatedFrames(analysis)
		framesTotal = len(analysis.Grids)
		strategy = "循环/运动感知"
	} else {
		indices = SelectAnimatedFrameIndices(totalFrames)
	}
	if len(indices) == 0 {
		return nil, fmt.Errorf("动态图未解析出可理解帧")
	}

	tmpDir, err := os.MkdirTemp("", "lunar_anim_frames_*")
	if err != nil {
		return nil, fmt.Errorf("创建临时帧目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pattern := filepath.Join(tmpDir, "frame-%03d.jpg")
	vf := fmt.Sprintf("scale='min(%d,iw)':-2", AnimatedFrameMaxDim)
	// 帧数已知（分析通道实测或探测成功）时用 select 表达式精确解码；
	// 否则解码前 animatedAnalyzeCap 帧再在内存中筛选
	knownCount := useSmart || (probeErr == nil && probe.Frames > 0)
	if knownCount {
		vf = fmt.Sprintf("select='%s',%s", buildAnimatedSelectExpr(indices), vf)
	}
	args := ffmpeg.KwArgs{
		"vf":       vf,
		"vsync":    "0",
		"qscale:v": "2",
		"loglevel": "error",
	}
	if knownCount {
		args["frames:v"] = strconv.Itoa(len(indices))
	} else {
		args["frames:v"] = strconv.Itoa(totalFrames)
	}
	stream := ffmpeg.Input(tempFile).Output(pattern, args)
	if *config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*config.FfmpegPath)
	}
	if err := stream.Run(); err != nil {
		return nil, fmt.Errorf("动态图解码失败: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(tmpDir, "frame-*.jpg"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("动态图未解码出可用帧")
	}
	sort.Strings(files)
	// 双重失败（无分析、无探测）时 ffmpeg 已输出全部帧，这里按同一规则补做选择
	if !knownCount {
		files = selectFileSubset(files, SelectAnimatedFrameIndices(len(files)))
	}

	// 输出帧数与选中下标数量一致时，时间标签可按原始位置换算；否则退化为序号
	exactMapping := len(files) == len(indices) && framesTotal > 0 && probe.Duration > 0
	frames := make([]MediaFrame, 0, len(files))
	for i, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil || len(data) == 0 {
			continue
		}
		timePoint := float64(i)
		if exactMapping {
			timePoint = probe.Duration * float64(indices[i]) / float64(framesTotal)
		}
		frames = append(frames, MediaFrame{
			Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data),
			Time:  timePoint,
		})
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("动态图未解码出可用帧")
	}

	logger.Info("MultimodalAnalysis", "AnimatedImageToFrames: %s(%d bytes, 共 %d 帧) → 解读 %d 帧（长边≤%d，%s）",
		format, len(imgData), framesTotal, len(frames), AnimatedFrameMaxDim, strategy)
	return []MediaFrameGroup{{Start: 0, End: probe.Duration, Frames: frames}}, nil
}

// animatedProbeInfo 动态图探测结果
type animatedProbeInfo struct {
	// Frames 原始帧数
	Frames int
	// Duration 原始时长（秒）
	Duration float64
}

// probeAnimatedInfo 用 ffprobe 读取动态图的总帧数与时长
func probeAnimatedInfo(inputFile string) (animatedProbeInfo, error) {
	data, err := ffmpeg.Probe(inputFile)
	if err != nil {
		return animatedProbeInfo{}, err
	}
	var output struct {
		Streams []struct {
			NbFrames     string `json:"nb_frames"`
			Duration     string `json:"duration"`
			AvgFrameRate string `json:"avg_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return animatedProbeInfo{}, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}
	if len(output.Streams) == 0 {
		return animatedProbeInfo{}, fmt.Errorf("ffprobe 输出无视频流")
	}
	stream := output.Streams[0]
	frames, _ := strconv.Atoi(stream.NbFrames)
	duration, _ := strconv.ParseFloat(stream.Duration, 64)
	if frames <= 0 && duration > 0 {
		if fps := parseFrameRate(stream.AvgFrameRate); fps > 0 {
			frames = int(math.Round(duration * fps))
		}
	}
	if frames <= 0 {
		return animatedProbeInfo{}, fmt.Errorf("无法确定动态图帧数")
	}
	return animatedProbeInfo{Frames: frames, Duration: duration}, nil
}

// parseFrameRate 解析 "25/2" 形式的帧率
func parseFrameRate(rate string) float64 {
	if rate == "" {
		return 0
	}
	parts := strings.Split(rate, "/")
	if len(parts) == 2 {
		numerator, _ := strconv.ParseFloat(parts[0], 64)
		denominator, _ := strconv.ParseFloat(parts[1], 64)
		if denominator > 0 {
			return numerator / denominator
		}
		return 0
	}
	value, _ := strconv.ParseFloat(rate, 64)
	return value
}

// buildAnimatedSelectExpr 把选中下标压缩为 ffmpeg select 表达式
func buildAnimatedSelectExpr(indices []int) string {
	if len(indices) == 0 {
		return "0"
	}
	ranges := make([][2]int, 0, len(indices))
	ranges = append(ranges, [2]int{indices[0], indices[0]})
	for _, index := range indices[1:] {
		last := &ranges[len(ranges)-1]
		if index == last[1]+1 {
			last[1] = index
			continue
		}
		ranges = append(ranges, [2]int{index, index})
	}
	parts := make([]string, 0, len(ranges))
	for _, span := range ranges {
		parts = append(parts, fmt.Sprintf("between(n\\,%d\\,%d)", span[0], span[1]))
	}
	return strings.Join(parts, "+")
}

// selectFileSubset 按下标挑选文件列表（探测失败的回退路径）
func selectFileSubset(files []string, indices []int) []string {
	out := make([]string, 0, len(indices))
	for _, index := range indices {
		if index >= 0 && index < len(files) {
			out = append(out, files[index])
		}
	}
	return out
}
