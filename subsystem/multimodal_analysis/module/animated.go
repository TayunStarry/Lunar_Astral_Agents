package module

// 动态图（GIF / APNG / 动态 WebP）检测与视频化
//
// 动态图编码为视频写入 llama-server 媒体目录，复用视频理解链路
// （llama-server 端 ffmpeg 抽帧 + 感知者角色），时序由视频采样点承载。
// 抽帧频率是 --video-fps（当前配置 0.5fps），数秒长的动态图直接转视频
// 只能抽到 1~2 帧，因此转码时按目标采样点数做等比例慢放（setpts）。
// 刻意不采用「循环拼接补时长」：当循环周期与抽帧周期接近时各采样点会落在
// 同一相位上（走样），退化成一张静态图；慢放则保证每个采样点落在动作的不同相位。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"image/gif"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

const (
	// AnimatedSampleFps llama-server 视频抽帧频率，需与 --video-fps 保持一致
	// （见 lunar_astral/model/llama/proxy.go）
	AnimatedSampleFps = 0.5
	// AnimatedTargetSamples 动态图视频化后的目标采样帧数：动作过程至少拆成这么多可辨识相位
	AnimatedTargetSamples = 12
	// AnimatedMaxSeconds 动态图视频化后的最大时长（秒）：不超过单个媒体片段上限，避免被切段
	AnimatedMaxSeconds = MediaChunkSeconds
	// AnimatedVideoFps 动态图视频化后的输出帧率：慢放后的时间轴本就稀疏，
	// 固定帧率只为让末帧铺满目标时长并提供确定可复现的抽帧网格
	AnimatedVideoFps = 10
)

// maxAnimatedBytes 动态图最大输入字节数（与静态图处理保持一致的上限）
const maxAnimatedBytes = 50 * 1024 * 1024

// IsAnimatedImage 判断图片数据是否为动态图（多帧 GIF / APNG / 动态 WebP）
// 非动态图（含单帧 GIF）返回 false，由静态图链路处理
func IsAnimatedImage(imgData []byte) bool {
	if len(imgData) == 0 || len(imgData) > maxAnimatedBytes {
		return false
	}
	switch detectImageFormat(imgData) {
	case "gif":
		// GIF 帧数需完整解码统计，单帧与双帧 GIF 按静态图处理
		gifImg, err := gif.DecodeAll(bytes.NewReader(imgData))
		if err != nil {
			return false
		}
		return len(gifImg.Image) > 2
	case "png":
		return isAPNG(imgData)
	case "webp":
		return isAnimatedWebP(imgData)
	default:
		return false
	}
}

// AnimatedImageToMedia 将动态图编码为慢放视频并写入 llama-server 媒体目录
//
// 返回媒体目录内的单个片段（文件名 + 起止时间），由 TypeScript 拼接 file:// 引用，
// 交给感知者角色走视频理解链路。文件名取输入内容哈希，同一张动态图重复处理直接覆盖。
func AnimatedImageToMedia(imgData []byte) ([]MediaSegment, error) {
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

	// 探测原始时长：失败时按最小时长处理（等价于最大慢放）
	duration, err := GetVideoDuration(tempFile)
	if err != nil {
		LoggerGeneral.Warn("MultimodalAnalysis", "AnimatedImageToMedia: 动态图时长探测失败(%v)，按最小时长处理", err)
		duration = 0
	}
	target := animatedTargetSeconds(duration)

	fileName := fmt.Sprintf("%x.mp4", sha1.Sum(imgData))
	outPath := filepath.Join(MediaDir(), fileName)
	if err := encodeAnimatedVideo(tempFile, outPath, duration, target); err != nil {
		return nil, err
	}

	LoggerGeneral.Info("MultimodalAnalysis", "AnimatedImageToMedia: %s(%d bytes, %.2fs) → %s(%.2fs, 慢放 %.2fx)",
		format, len(imgData), duration, fileName, target, animatedSlowFactor(duration, target))
	return []MediaSegment{{File: fileName, Start: 0, End: target}}, nil
}

// animatedTargetSeconds 计算视频化目标时长
// 采样点数不足目标值时补足到 AnimatedTargetSamples/AnimatedSampleFps，
// 原始时长已足够时保持原速，上限为 AnimatedMaxSeconds
func animatedTargetSeconds(duration float64) float64 {
	want := float64(AnimatedTargetSamples) / AnimatedSampleFps
	if duration <= 0 || duration < want {
		return math.Min(want, AnimatedMaxSeconds)
	}
	return math.Min(duration, AnimatedMaxSeconds)
}

// animatedSlowFactor 计算慢放倍数（不小于 1，即只慢放不加速）
func animatedSlowFactor(duration, target float64) float64 {
	if duration <= 0 {
		return 1
	}
	if factor := target / duration; factor > 1 {
		return factor
	}
	return 1
}

// encodeAnimatedVideo 将动态图编码为 H.264 视频：
// setpts 慢放至目标时长并输出固定帧率，尺寸取偶、像素格式转 yuv420p（H.264 要求），仅保留视频流
//
// 固定帧率（-r）不只是为了规整：setpts 只拉伸时间戳、不改末帧自身时长
// （GIF 末帧时长即其帧延迟），实测 1 秒 10 帧 GIF 慢放后整条视频只有 21.7 秒而非 24 秒，
// 抽帧采样点随之减少。CFR 输出会把每帧按 1/fps 铺满时间线，末帧自然延伸到目标时长，
// 同时让 llama-server 端按固定网格抽帧，采样点确定可复现。
func encodeAnimatedVideo(inputFile, outFile string, duration, target float64) error {
	slowFactor := animatedSlowFactor(duration, target)
	filters := []string{
		fmt.Sprintf("setpts=%.4f*PTS", slowFactor),
		// H.264 需要偶数宽高与 yuv420p 像素格式
		"scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"format=yuv420p",
	}
	base := ffmpeg.KwArgs{
		"vf":       strings.Join(filters, ","),
		"t":        fmt.Sprintf("%.3f", target),
		"r":        fmt.Sprintf("%d", AnimatedVideoFps),
		"map":      "0:v:0",
		"movflags": "+faststart",
		"loglevel": "error",
	}

	// 首选 libx264；构建环境缺少该编码器时退到内置 mpeg4（llama-server 端 ffmpeg 均可解码）
	encoders := []ffmpeg.KwArgs{
		{"c:v": "libx264", "preset": "veryfast", "crf": "23"},
		{"c:v": "mpeg4", "q:v": "5"},
	}
	var lastErr error
	for _, enc := range encoders {
		args := ffmpeg.KwArgs{}
		for key, value := range base {
			args[key] = value
		}
		for key, value := range enc {
			args[key] = value
		}
		// ffmpeg-go 无法正确传递 -y 布尔参数，改为提前删除旧文件
		os.Remove(outFile)
		stream := ffmpeg.Input(inputFile).Output(outFile, args)
		if *GeneralConfig.FfmpegPath != "" {
			stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
		}
		if err := stream.Run(); err != nil {
			lastErr = err
			continue
		}
		if info, err := os.Stat(outFile); err != nil || info.Size() == 0 {
			if err == nil {
				err = fmt.Errorf("输出文件为空")
			}
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("动态图转视频失败: %w", lastErr)
}

// writeTempImage 将图片数据写入带正确扩展名的临时文件，供 ffmpeg 按扩展名识别
func writeTempImage(imgData []byte, format string) (string, error) {
	ext := ".bin"
	switch format {
	case "gif":
		ext = ".gif"
	case "png":
		ext = ".png"
	case "webp":
		ext = ".webp"
	}
	tempFile, err := os.CreateTemp("", "lunar_animated_*"+ext)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := tempFile.Write(imgData); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tempFile.Close()
	return tempFile.Name(), nil
}

// ImageSourceToBytes 按来源读取图片数据：支持 HTTP(S) URL、data:image;base64 URI 与本地文件路径
// 供适配器在 TypeScript 传入图片地址（而非字节）时使用
func ImageSourceToBytes(source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("图片地址为空")
	}
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return downloadImageBytes(source)
	case strings.HasPrefix(source, "data:"):
		parts := strings.SplitN(source, ",", 2)
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
			return nil, fmt.Errorf("无效的 data URI 格式")
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("base64 解码失败: %w", err)
		}
		return data, nil
	default:
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("读取图片文件失败: %w", err)
		}
		return data, nil
	}
}

// imageSourceClient 图片下载客户端
//   - 60 秒超时：远端无响应时不得阻塞智能体主循环（图片体积远小于模型/媒体文件）
//   - 跳过 TLS 校验：与 lunar_goja 的 fetch 默认策略一致，兼容自签名图床
var imageSourceClient = &http.Client{
	Timeout:   60 * time.Second,
	Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
}

// downloadImageBytes 下载图片到内存，超过 maxAnimatedBytes 时中止
func downloadImageBytes(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	// 部分图床会拒绝无 UA 的请求
	req.Header.Set("User-Agent", "LunarAstral/1.0")

	resp, err := imageSourceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载图片失败，状态码 %d", resp.StatusCode)
	}

	// 多读 1 字节用于判断是否超限
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnimatedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if len(data) > maxAnimatedBytes {
		return nil, fmt.Errorf("图片数据过大，超过 %dMB 限制", maxAnimatedBytes/1024/1024)
	}
	return data, nil
}

// isAPNG 检测PNG数据是否为动态PNG（APNG）
// 通过扫描PNG chunk，检测是否存在acTL（Animation Control）chunk
func isAPNG(data []byte) bool {
	// PNG签名: 8字节
	if len(data) < 8+4+4+4 {
		return false
	}
	// 跳过PNG签名(8字节)，从第一个chunk开始扫描
	offset := 8
	// 限制扫描100个chunk，防止恶意数据
	for i := 0; i < 100 && offset+12 <= len(data); i++ {
		// chunk结构: 4字节长度 + 4字节类型 + 数据 + 4字节CRC
		chunkLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		chunkType := string(data[offset+4 : offset+8])

		if chunkType == "acTL" {
			return true
		}
		// 跳转到下一个chunk: 4(length) + 4(type) + chunkLen(data) + 4(CRC)
		offset += 12 + chunkLen
	}
	return false
}

// isAnimatedWebP 检测WebP数据是否为动态WebP
// 通过扫描RIFF容器中的chunk，检测是否存在ANIM chunk
func isAnimatedWebP(data []byte) bool {
	// RIFF容器最小长度: RIFF(4) + size(4) + WEBP(4) + chunk(4+4) = 20
	if len(data) < 20 {
		return false
	}
	// 检查RIFF头
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}
	// 从第一个chunk开始扫描（偏移12字节跳过RIFF头+WEBP标识）
	offset := 12
	fileEnd := 8 + (int(data[4]) | int(data[5])<<8 | int(data[6])<<16 | int(data[7])<<24)

	// 限制扫描100个chunk
	for i := 0; i < 100 && offset+8 <= len(data) && offset+8 <= fileEnd; i++ {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(data[offset+4]) | int(data[offset+5])<<8 | int(data[offset+6])<<16 | int(data[offset+7])<<24

		if chunkType == "ANIM" {
			return true
		}
		// 跳转到下一个chunk（注意chunk大小奇偶填充）
		offset += 8 + chunkSize
		if chunkSize%2 == 1 {
			offset++ // 奇数大小会有1字节填充
		}
	}
	return false
}
