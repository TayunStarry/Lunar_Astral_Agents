package napcat

// audio_source.go 演奏音频地址归一（传输适配）。
//
// 只负责把内部音频地址翻译成 NapCat 能消费的 file 值或用户可访问的地址；
// 发不出去之后降级到哪一级，由调用方的投递策略决定。

import (
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// AudioSource 把内部音频地址归一为 NapCat 可消费的 file 值：
// 本机内容仓库路径优先给绝对路径（避免多余下载），远程地址原样交给 NapCat。
func AudioSource(audio string) (string, bool) {
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

// LocalAudioFile 取本机音频绝对路径（远程地址无法走文件上传时返回 false）
func LocalAudioFile(audio string) (string, bool) {
	source, ok := AudioSource(audio)
	if !ok {
		return "", false
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "", false
	}
	return source, true
}

// AudioLink 构造可供用户直接访问的音频地址（用于降级文字说明）
func AudioLink(audio string) string {
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
