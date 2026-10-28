package multimodal

// watch.go 统一视觉观看管线：视频与动图共用同一套切片与抽帧规则。
//
// 规则只有一份：
//   - 视频按 MediaChunkSeconds（60 秒）切片，丢弃不足 1 秒的尾巴，最多 WatchMaxSlices 片；
//   - 动图没有独立链路，整体视为一个切片（起点 0，终点为动画时长）；
//   - 每个切片在 [start, end) 上等距取 WatchFrameMax 个时间点抽帧，
//     长边不超过 MaxMediaWidth，JPEG 编码为 data URI 直接作为 image_url 提交。
//
// 与旧「1fps 抽满 60 帧再均匀挑 12 帧」的取帧位置一致（0、5、10…55 秒），
// 但只解码真正要用的帧：抽帧体系唯一、结果可预测、开销下降一个量级。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// SplitMediaSegments 计算观看切片：每 MediaChunkSeconds 秒一片，
// 丢弃不足 1 秒的尾巴（下载截断常见）；maxSlices > 0 时从开头截取该片数，
// maxSlices <= 0 时返回全部切片（观看成本上限由调用方用 pickSegments 处理）。
func SplitMediaSegments(duration float64, maxSlices int) []MediaSegment {
	if duration <= 0 {
		return nil
	}
	count := int(math.Ceil(duration / MediaChunkSeconds))
	if count < 1 {
		count = 1
	}
	if maxSlices <= 0 {
		maxSlices = count
	}
	segments := make([]MediaSegment, 0, count)
	for i := 0; i < count && i < maxSlices; i++ {
		start := float64(i) * MediaChunkSeconds
		end := math.Min(start+MediaChunkSeconds, duration)
		if end-start < 1.0 {
			continue
		}
		segments = append(segments, MediaSegment{Start: start, End: end})
	}
	if len(segments) == 0 {
		segments = append(segments, MediaSegment{Start: 0, End: duration})
	}
	return segments
}

// pickSegments 切片数超过上限时等距选片（恒含首片与末片）。
//
// 长视频只看开头会让「整段影像的印象」失真，所以上限只限制观看成本、不限制覆盖范围。
func pickSegments(segments []MediaSegment, max int) []MediaSegment {
	if max <= 0 || len(segments) <= max {
		return segments
	}
	picked := make([]MediaSegment, 0, max)
	seen := make(map[int]bool, max)
	for i := 0; i < max; i++ {
		index := (i * (len(segments) - 1)) / (max - 1)
		if seen[index] {
			continue
		}
		seen[index] = true
		picked = append(picked, segments[index])
	}
	return picked
}

// SegmentImages 抽取一个切片的采样帧：区间内等距取 frameMax 个时间点，
// 长边压到 MaxMediaWidth 后编码为 JPEG data URI；个别帧失败只跳过，不中断整片。
func SegmentImages(localPath string, segment MediaSegment, frameMax int) ([]string, error) {
	if frameMax <= 0 {
		frameMax = WatchFrameMax
	}
	span := segment.End - segment.Start
	if span <= 0 {
		return nil, fmt.Errorf("切片时间区间无效: %.3f-%.3f", segment.Start, segment.End)
	}
	quality := config.JPEGQuality
	images := make([]string, 0, frameMax)
	for i := 0; i < frameMax; i++ {
		// 取 span/frameMax 的步长而不是 span/(frameMax-1)：末帧落在区间内，
		// 不会去取片段结束点（该点已属于下一片）
		t := segment.Start + span*float64(i)/float64(frameMax)
		img, err := ExtractVideoFrameAt(localPath, t, MaxMediaWidth)
		if err != nil {
			logger.Warn(LogModuleMedia, "切片第%d帧(%s)抽取失败，跳过: %v", i+1, FormatSeconds(t), err)
			continue
		}
		buf := &bytes.Buffer{}
		if err := encodeImage(buf, img, "jpeg", quality); err != nil {
			logger.Warn(LogModuleMedia, "切片第%d帧(%s)编码失败，跳过: %v", i+1, FormatSeconds(t), err)
			continue
		}
		images = append(images, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(buf.Bytes()))
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("切片 %s-%s 未抽取到可用画面",
			FormatSeconds(segment.Start), FormatSeconds(segment.End))
	}
	return images, nil
}

// VideoToWatchSlices 视频 → 观看切片（全部在内存中，不写媒体目录）。
//
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
func VideoToWatchSlices(inputFile string, maxSlices, frameMax int) ([]WatchSlice, error) {
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

	segments := pickSegments(SplitMediaSegments(duration, 0), maxSlices)
	slices := make([]WatchSlice, 0, len(segments))
	for _, segment := range segments {
		images, frameErr := SegmentImages(localPath, segment, frameMax)
		if frameErr != nil {
			logger.Warn(LogModuleMedia, "视频切片 %s-%s 抽帧失败，跳过: %v",
				FormatSeconds(segment.Start), FormatSeconds(segment.End), frameErr)
			continue
		}
		slices = append(slices, WatchSlice{
			Index:  len(slices) + 1,
			Start:  segment.Start,
			End:    segment.End,
			Images: images,
		})
	}
	if len(slices) == 0 {
		return nil, fmt.Errorf("未能从 %s 中抽取到可用画面", inputFile)
	}
	logger.Info(LogModuleMedia, "VideoToWatchSlices: 时长 %.2fs → %d 片（每片 %d 帧）, 来源=%s",
		duration, len(slices), WatchFrameMax, inputFile)
	return slices, nil
}

// AnimatedToWatchSlice 动态图 → 单个观看切片。
//
// 动图不再走「循环裁剪 + 运动窗口 + 智能选帧」的独立链路：它就是一段很短的影像，
// 起点 0、终点为动画时长，与视频切片共用 SegmentImages 的等距抽帧。
func AnimatedToWatchSlice(imgData []byte, frameMax int) (WatchSlice, error) {
	if len(imgData) == 0 {
		return WatchSlice{}, fmt.Errorf("动态图数据为空")
	}
	if len(imgData) > maxAnimatedBytes {
		return WatchSlice{}, fmt.Errorf("动态图数据过大，超过50MB限制")
	}
	if !IsAnimatedImage(imgData) {
		return WatchSlice{}, fmt.Errorf("不是动态图（多帧 GIF / APNG / 动态 WebP）")
	}
	// ffmpeg 需要按扩展名识别解复用器，管道输入对动态 WebP 不可靠，先落临时文件
	format := detectImageFormat(imgData)
	tempFile, err := writeTempImage(imgData, format)
	if err != nil {
		return WatchSlice{}, err
	}
	defer os.Remove(tempFile)

	duration, err := animatedDuration(tempFile)
	if err != nil {
		return WatchSlice{}, err
	}
	images, err := SegmentImages(tempFile, MediaSegment{Start: 0, End: duration}, frameMax)
	if err != nil {
		return WatchSlice{}, err
	}
	logger.Info(LogModuleMedia, "AnimatedToWatchSlice: %s(%d bytes, %.2fs) → %d 帧（长边≤%d）",
		format, len(imgData), duration, len(images), MaxMediaWidth)
	return WatchSlice{Index: 1, Start: 0, End: duration, Images: images}, nil
}

// SampleImages 从较长的帧序列里等距挑出最多 max 张（帧数不足时原样返回）。
// 只在「整段影像一次性提问」时使用，常规观看由 SegmentImages 直接控制帧数。
func SampleImages(images []string, max int) []string {
	if max <= 0 || len(images) <= max {
		return images
	}
	out := make([]string, 0, max)
	step := float64(len(images)) / float64(max)
	for i := 0; i < max; i++ {
		index := int(float64(i) * step)
		if index >= len(images) {
			index = len(images) - 1
		}
		out = append(out, images[index])
	}
	return out
}

// animatedDuration 读取动态图时长；容器没写时长时按「帧数 / 帧率」换算。
func animatedDuration(inputFile string) (float64, error) {
	if duration, err := GetVideoDuration(inputFile); err == nil && duration > 0 {
		return duration, nil
	}
	data, err := ffmpeg.Probe(inputFile)
	if err != nil {
		return 0, fmt.Errorf("探测动态图失败: %w", err)
	}
	var output struct {
		Streams []struct {
			NbFrames     string `json:"nb_frames"`
			AvgFrameRate string `json:"avg_frame_rate"`
			RFrameRate   string `json:"r_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, fmt.Errorf("解析动态图探测结果失败: %w", err)
	}
	if len(output.Streams) > 0 {
		stream := output.Streams[0]
		frames, _ := strconv.Atoi(stream.NbFrames)
		fps := parseFrameRate(stream.AvgFrameRate)
		if fps <= 0 {
			fps = parseFrameRate(stream.RFrameRate)
		}
		if frames > 0 && fps > 0 {
			return float64(frames) / fps, nil
		}
	}
	return 0, fmt.Errorf("无法确定动态图时长")
}

// parseFrameRate 解析 "25/2" 形式的帧率
func parseFrameRate(rate string) float64 {
	parts := strings.Split(strings.TrimSpace(rate), "/")
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
