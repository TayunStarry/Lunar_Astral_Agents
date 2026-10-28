package multimodal

// video_media.go 视频来源本地化。
//
// 旧的 VideoToMedia（把视频切进 multimedia/footage 再交给 llama-server 按帧解码）
// 已随抽帧理解链路一并移除：现在统一由 watch.go 现场抽帧，不再落盘片段文件。

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

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
