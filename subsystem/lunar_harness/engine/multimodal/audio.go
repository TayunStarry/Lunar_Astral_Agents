package multimodal

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/netsource"
)

// AudioToWavBase64 将音频本地化并转换为 16kHz 单声道 WAV，返回 base64 编码
//
// 输入支持 HTTP(S) URL、data:audio/xxx;base64 URI 与本地文件路径。
// 16kHz 单声道是多模态模型音频编码器的标准输入格式，同时压缩传输体积。
func AudioToWavBase64(inputFile string) (string, error) {
	// 本地化输入：本机服务地址直接读盘，远程地址才下载到临时文件
	localPath, cleanup, err := localizeAudioInput(inputFile)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// 校验本地文件存在
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("音频文件不存在: %s", localPath)
	}

	// 转换为 16kHz 单声道 WAV（ffmpeg-go 无法传递 -y 布尔参数，改为提前删除旧输出）
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("lunar_audio_%d.wav", os.Getpid()))
	os.Remove(outPath)
	stream := ffmpeg.Input(localPath).Output(outPath, ffmpeg.KwArgs{
		"ac":       "1",
		"ar":       "16000",
		"loglevel": "error",
	})
	if config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(config.FfmpegPath)
	}
	if err := stream.Run(); err != nil {
		return "", fmt.Errorf("音频转码失败: %w", err)
	}
	defer os.Remove(outPath)

	data, err := os.ReadFile(outPath)
	if err != nil {
		return "", fmt.Errorf("读取转码结果失败: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("音频转码输出为空")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// localizeAudioInput 将音频输入归一为本地文件
//
// 返回的 cleanup 仅在输入被落地为临时文件时非空；本机服务地址直接复用已存在的
// local_data 文件，cleanup 为 nil，绝不能按临时下载件删除。
func localizeAudioInput(inputFile string) (string, func(), error) {
	switch {
	case strings.HasPrefix(inputFile, "http://") || strings.HasPrefix(inputFile, "https://"):
		if local, ok := netsource.LocalPath(inputFile); ok {
			return local, nil, nil
		}
		tempFile, err := downloadAudioToTemp(inputFile)
		if err != nil {
			return "", nil, fmt.Errorf("下载音频文件失败: %w", err)
		}
		return tempFile, func() { os.Remove(tempFile) }, nil
	case strings.HasPrefix(inputFile, "data:audio/"):
		tempFile, err := dataAudioURIToTempFile(inputFile)
		if err != nil {
			return "", nil, fmt.Errorf("解码音频数据失败: %w", err)
		}
		return tempFile, func() { os.Remove(tempFile) }, nil
	default:
		return inputFile, nil, nil
	}
}

// downloadAudioToTemp 回源下载音频到临时文件，带超时与大小上限
func downloadAudioToTemp(rawURL string) (string, error) {
	resp, err := netsource.Open(rawURL, audioDownloadTimeout)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp("", "lunar_audio_*"+audioTempExtension(rawURL))
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	name := tempFile.Name()
	if err := netsource.CopyLimited(tempFile, resp.Body, maxAudioDownloadBytes); err != nil {
		tempFile.Close()
		os.Remove(name)
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("关闭临时文件失败: %w", err)
	}
	return name, nil
}

// audioTempExtension 从 URL 路径推断临时文件扩展名（辅助 ffmpeg 识别）
func audioTempExtension(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ".bin"
	}
	ext := strings.ToLower(path.Ext(parsed.Path))
	if ext == "" || len(ext) > 8 {
		return ".bin"
	}
	return ext
}

// dataAudioURIToTempFile 将 data:audio/xxx;base64 URI 解码为本地临时文件（保留原始扩展名以辅助格式识别）
func dataAudioURIToTempFile(uri string) (string, error) {
	parts := strings.SplitN(uri, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
		return "", fmt.Errorf("无效的 data URI 格式")
	}
	// 从 MIME 前缀推断扩展名（data:audio/mpeg → .mp3）
	ext := ".bin"
	mime := strings.TrimPrefix(parts[0], "data:")
	switch mime {
	case "audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave":
		ext = ".wav"
	case "audio/mpeg":
		ext = ".mp3"
	case "audio/flac":
		ext = ".flac"
	case "audio/ogg":
		ext = ".ogg"
	case "audio/amr":
		ext = ".amr"
	case "audio/silk", "audio/x-silk":
		ext = ".silk"
	default:
		if idx := strings.Index(mime, "/"); idx >= 0 {
			ext = "." + mime[idx+1:]
		}
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	tempFile, err := os.CreateTemp("", "lunar_audio_*"+ext)
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
