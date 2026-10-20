package multimodal

import (
	"LunarSubsystem/GeneralConfig"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MediaChunkSeconds 单个媒体片段的最大时长（秒）。
// llama-server 按视频抽帧频率（--video-fps，配置 1fps）抽取视觉 token，
// 80 秒片段 = 80 帧；长边 ≤640 的单帧实测 Gemma 多模态约 106~171 token，
// 单片段视觉开销 ≤13.7k，叠加提示词与输出后仍落在 20480 上下文内。
const MediaChunkSeconds = 80.0

// MaxMediaWidth 媒体片段的最大长边（像素，宽或高均受限）。
// llama-server 的多模态批处理要求非因果注意力块整体落入单个 ubatch，
// 高分辨率视频单帧的视觉 token 数会超出 n_ubatch 上限，触发 GGML 断言使多模态实例崩溃；
// 长边压到 640px 后每帧 token 数降至安全范围（实测 Gemma 多模态 106~171 token/帧）。
const MaxMediaWidth = 640

// NormalizedFPS 重编码片段的输出帧率：低帧率仅用于压缩重编码耗时，
// 实际抽帧频率由 llama-server 端 --video-fps 决定，与此处互不影响
const NormalizedFPS = 10

// MediaSegment 媒体目录内的视频片段信息
type MediaSegment struct {
	// File 媒体目录内的文件名（通过 file:// 引用）
	File string `json:"file"`
	// Start 片段起始时间（秒）
	Start float64 `json:"start"`
	// End 片段结束时间（秒）
	End float64 `json:"end"`
}

// localizeVideoSource 将视频来源本地化为可读路径：
// HTTP(S) URL 下载到临时文件、data:video URI 解码到临时文件、其余按本地路径直接使用。
// 返回的 cleanup 供调用方 defer 调用（本地路径时为空操作）；文件不存在或本地化失败时返回错误。
func localizeVideoSource(inputFile string) (string, func(), error) {
	localPath := inputFile
	cleanup := func() {}
	if strings.HasPrefix(inputFile, "http://") || strings.HasPrefix(inputFile, "https://") {
		tempFile, err := downloadToTempFile(inputFile)
		if err != nil {
			return "", cleanup, fmt.Errorf("下载视频文件失败: %w", err)
		}
		localPath, cleanup = tempFile, func() { os.Remove(tempFile) }
	} else if strings.HasPrefix(inputFile, "data:video/") {
		tempFile, err := dataURIToTempFile(inputFile)
		if err != nil {
			return "", cleanup, fmt.Errorf("解码视频数据失败: %w", err)
		}
		localPath, cleanup = tempFile, func() { os.Remove(tempFile) }
	}
	// 校验本地文件存在（临时文件此时已产生，失败时一并清理）
	if _, err := os.Stat(localPath); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("视频文件不存在: %s", localPath)
	}
	return localPath, cleanup, nil
}

// VideoToMedia 将视频本地化并复制/分段到 llama-server 媒体目录（multimedia/footage）
//
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
// 时长不超过 MediaChunkSeconds 的视频直接复制为单个媒体文件；
// 更长的视频使用 FFmpeg -c copy 快速切分为多个片段。
// 返回媒体目录内的片段列表（footage/ 子路径 + 起止时间），由 TypeScript 拼接 file:// 引用。
// 不依赖服务端解码视频的「图片序列帧」模式见 video_frame.go 的 VideoToFrames。
func VideoToMedia(inputFile string) ([]MediaSegment, error) {
	// 本地化输入：URL / data URI 落到临时文件，本地路径直接使用
	localPath, cleanup, err := localizeVideoSource(inputFile)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// 探测视频时长
	duration, err := GetVideoDuration(localPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	if duration > 3600 {
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	// 以输入源稳定哈希作为媒体文件名，同一视频重复处理直接覆盖
	hash := fmt.Sprintf("%x", sha1.Sum([]byte(inputFile)))
	// 长边（宽或高）超过安全上限、探测失败或格式不受支持的源必须重编码归一化：
	// 防止 llama-server 端 n_ubatch 断言崩溃，同时把单帧视觉 token 压到 ≤171
	width, height, dimErr := GetVideoDimensions(localPath)
	needNormalize := dimErr != nil || width > MaxMediaWidth || height > MaxMediaWidth
	ext := strings.ToLower(filepath.Ext(localPath))
	if needNormalize || !IsSupportedVideoFormat("v"+ext) {
		ext = ".mp4"
	}

	// 计算分段数
	segments := max(int((duration-0.001)/MediaChunkSeconds)+1, 1)

	// 片段归档至 multimedia/footage 子目录；llama-server 的 media-path 指向 multimedia 根，
	// 返回的文件名需携带 footage/ 前缀才能被 file:// 引用正确解析
	mediaDir := *GeneralConfig.LocalDir + "/multimedia/footage"
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return nil, fmt.Errorf("创建媒体片段目录失败: %w", err)
	}
	result := make([]MediaSegment, 0, segments)
	for i := range segments {
		start := float64(i) * MediaChunkSeconds
		end := start + MediaChunkSeconds
		if end > duration {
			end = duration
		}
		// 单片段视频直接复制原始文件，多片段才需要 FFmpeg 切分
		fileName := fmt.Sprintf("footage/%s%s", hash, ext)
		if segments > 1 {
			fileName = fmt.Sprintf("footage/%s_seg%d%s", hash, i, ext)
		}
		outPath := filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(fileName))
		// 音轨一律不进入切片（声音与音乐对视频理解无效）：
		// 重编码路径本来就只保留视频流，流复制路径经 map 0:v:0 只映射视频流
		var err error
		if needNormalize {
			// 归一化重编码：输入侧定位片段起点，重编码后边界逐帧精确
			err = normalizeVideoSegment(localPath, outPath, start, end-start)
		} else {
			// 已达安全分辨率的源：流复制切分（含单片段场景，顺带剥离音轨）
			err = cutVideoSegment(localPath, outPath, start, end-start)
		}
		if err != nil {
			return nil, fmt.Errorf("写入视频片段%d失败: %w", i, err)
		}
		result = append(result, MediaSegment{File: fileName, Start: start, End: end})
	}
	return result, nil
}

// cutVideoSegment 使用 FFmpeg 流复制切分视频片段（不重编码，速度快，仅用于已达安全分辨率的源）
//
// -ss 必须放在输入侧（-i 之前）：流复制不重编码，输出侧 -ss 只按时间戳截包，
// 视频要等到切点后的下一个关键帧才有画面，音频却立即开始，导致片段开头
// 一大段黑屏且音画错位；输入侧定位直接跳到切点前最近的关键帧，片段以关键帧开头，
// 播放器可立即解码（实际起点可能比请求值提前最多一个 GOP，属可接受误差）。
// map 0:v:0 只保留首条视频流：音轨与字幕不进入切片（对视频理解无效）。
func cutVideoSegment(inputFile string, outFile string, start float64, duration float64) error {
	// FFmpeg 覆盖输出需要 -y 标志，ffmpeg-go 无法正确传递布尔参数，改为提前删除旧文件
	os.Remove(outFile)
	input := ffmpeg.Input(inputFile)
	if start > 0 {
		input = ffmpeg.Input(inputFile, ffmpeg.KwArgs{"ss": fmt.Sprintf("%.3f", start)})
	}
	stream := input.Output(outFile, ffmpeg.KwArgs{
		"t":                 fmt.Sprintf("%.3f", duration),
		"c":                 "copy",
		"map":               "0:v:0",
		"avoid_negative_ts": "make_zero",
		"movflags":          "+faststart",
		"loglevel":          "error",
	})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	return stream.Run()
}

// normalizeVideoSegment 重编码视频片段到安全规格（长边 ≤ MaxMediaWidth、帧率 NormalizedFPS、H.264/yuv420p、仅视频流）
//
// 输入侧 -ss 定位片段起点（重编码路径下高效且边界逐帧精确）；
// 缩放按长边封顶（宽或高均受限，竖屏视频同样覆盖），只缩不放，
// 缩放表达式中的逗号需转义，避免被 FFmpeg 当作滤镜分隔符。
func normalizeVideoSegment(inputFile string, outFile string, start float64, duration float64) error {
	// FFmpeg 覆盖输出需要 -y 标志，ffmpeg-go 无法正确传递布尔参数，改为提前删除旧文件
	os.Remove(outFile)
	input := ffmpeg.Input(inputFile)
	if start > 0 {
		input = ffmpeg.Input(inputFile, ffmpeg.KwArgs{"ss": fmt.Sprintf("%.3f", start)})
	}
	stream := input.Output(outFile, ffmpeg.KwArgs{
		"t": fmt.Sprintf("%.3f", duration),
		"vf": fmt.Sprintf("scale=trunc(min(%d\\,iw)/2)*2:trunc(min(%d\\,ih)/2)*2:force_original_aspect_ratio=decrease,fps=%d,format=yuv420p",
			MaxMediaWidth, MaxMediaWidth, NormalizedFPS),
		"c:v":      "libx264",
		"preset":   "veryfast",
		"crf":      "23",
		"map":      "0:v:0",
		"movflags": "+faststart",
		"loglevel": "error",
	})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	if err := stream.Run(); err != nil {
		return err
	}
	// 校验输出非空，防止半截文件进入媒体目录
	if info, err := os.Stat(outFile); err != nil {
		return fmt.Errorf("输出文件缺失: %w", err)
	} else if info.Size() == 0 {
		return fmt.Errorf("输出文件为空")
	}
	return nil
}

// GetVideoDimensions 获取视频首条视频流的宽高（像素），无视频流或探测失败时返回错误
func GetVideoDimensions(inputFile string) (int, int, error) {
	data, err := ffmpeg.Probe(inputFile)
	if err != nil {
		return 0, 0, err
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, 0, fmt.Errorf("解析ffprobe输出失败: %w", err)
	}
	streams, ok := output["streams"].([]any)
	if !ok {
		return 0, 0, fmt.Errorf("ffprobe输出中无streams字段")
	}
	for _, s := range streams {
		if m, ok := s.(map[string]any); ok && m["codec_type"] == "video" {
			w, hasW := m["width"].(float64)
			h, hasH := m["height"].(float64)
			if hasW && hasH && w > 0 && h > 0 {
				return int(w), int(h), nil
			}
		}
	}
	return 0, 0, fmt.Errorf("未找到视频流")
}

// dataURIToTempFile 将 data:video/xxx;base64 URI 解码为本地临时文件
func dataURIToTempFile(uri string) (string, error) {
	parts := strings.SplitN(uri, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
		return "", fmt.Errorf("无效的 data URI 格式")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	tempFile, err := os.CreateTemp("", "video_media_*.mp4")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tempFile.Close()
	return tempFile.Name(), nil
}
