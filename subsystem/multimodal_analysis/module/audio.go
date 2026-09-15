package module

import (
	"LunarSubsystem/GeneralConfig"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// AudioToWavBase64 将音频本地化并转换为 16kHz 单声道 WAV，返回 base64 编码
//
// 输入支持 HTTP(S) URL、data:audio/xxx;base64 URI 与本地文件路径。
// 16kHz 单声道是多模态模型音频编码器的标准输入格式，同时压缩传输体积。
func AudioToWavBase64(inputFile string) (string, error) {
	// 本地化输入：非本地路径先落到临时文件
	localPath := inputFile
	isTemp := false
	if strings.HasPrefix(inputFile, "http://") || strings.HasPrefix(inputFile, "https://") {
		tempFile, err := downloadToTempFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("下载音频文件失败: %w", err)
		}
		localPath, isTemp = tempFile, true
	} else if strings.HasPrefix(inputFile, "data:audio/") {
		tempFile, err := dataAudioURIToTempFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("解码音频数据失败: %w", err)
		}
		localPath, isTemp = tempFile, true
	}
	if isTemp {
		defer os.Remove(localPath)
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
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
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
