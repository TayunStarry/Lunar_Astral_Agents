package napcat

// audio_source.go 演奏音频地址归一与降级辅助。

import (
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// audioMediaSource 把内部音频地址归一为 NapCat 可消费的 file 值：
// 本机内容仓库路径优先给绝对路径（避免多余下载），远程地址原样交给 NapCat。
func audioMediaSource(audio string) (string, bool) {
	raw := strings.TrimSpace(audio)
	if raw == "" {
		return "", false
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw, true
	}
	if path, ok := localContentPath(raw); ok {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Size() > 0 {
			return path, true
		}
	}
	if strings.HasPrefix(raw, "/") {
		return config.LocalBaseURL() + raw, true
	}
	return "", false
}

// localAudioFile 取本机音频绝对路径（远程地址无法走文件上传时返回 false）
func localAudioFile(audio string) (string, bool) {
	source, ok := audioMediaSource(audio)
	if !ok {
		return "", false
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "", false
	}
	return source, true
}

// audioLink 构造可供用户直接访问的音频地址（用于降级文字说明）
func audioLink(audio string) string {
	raw := strings.TrimSpace(audio)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return config.LocalBaseURL() + raw
	}
	if rel, ok := localContentRel(raw); ok {
		return config.LocalBaseURL() + "/file/read/" + filepath.ToSlash(rel)
	}
	return ""
}

// fileSizeOf 返回本机文件大小，失败时返回 0
func fileSizeOf(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}
