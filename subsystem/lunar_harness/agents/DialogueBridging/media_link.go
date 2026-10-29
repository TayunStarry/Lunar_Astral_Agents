package DialogueBridging

// media_link.go 渠道媒体段的链接句柄化
//
// 视频统一走 @U 链接句柄（句柄本体就是地址，不再下载后压成 @V 归档副本）；
// 本地文件形态的视频先落到 multimedia/ 再以本机读取地址登记，保证模型可回引。
//
// 同一个视频内容按 sha256 前 16 位命名：重复发送/转发会复用同一路径与同一 @U 句柄，
// 不再出现「每发一次就多一个句柄」的累加。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// channelFileReadPath 内容仓库的读取端点（与宿主 /file/read/ 对齐）
const channelFileReadPath = "/file/read/"

// mediaLinkText 渠道媒体段 → 「[@U 句柄] 文件名」；不可登记时退化为文件名
//
// 视频先尝试下载/解码并落到 multimedia/，用内容哈希命名，再以本机读取地址登记；
// 这样模型能凭 @U 句柄回看同一段视频，重复内容也不会重复占句柄。
func mediaLinkText(source, name string) string {
	raw := strings.TrimSpace(source)
	if raw != "" {
		if data, err := fetchFileBytes(raw); err == nil && len(data) > 0 {
			if localURL := saveVideoFileURL(name, data); localURL != "" {
				if res, admitErr := resource.AdmitURL(resource.URLAdmitInput{
					URL:      localURL,
					Name:     name,
					Title:    name,
					Origin:   resource.OriginChannel,
					KindHint: resource.KindVideo,
				}); admitErr == nil && res.Handle != "" {
					return "[" + res.Handle + "] " + name + " "
				}
			}
		} else if err != nil {
			logger.SubWarn("LunarCore", "Napcat", "视频下载失败（%s）: %v", name, err)
		}
	}
	// 下载失败时退回远程地址登记：句柄仍可回引，只是没有本地副本可供解读。
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if res, err := resource.AdmitURL(resource.URLAdmitInput{
			URL:      raw,
			KindHint: resource.KindVideo,
			Name:     name,
			Origin:   resource.OriginChannel,
		}); err == nil && res.Handle != "" {
			return "[" + res.Handle + "] " + name + " "
		}
	}
	return name + " "
}

// saveVideoFileURL 把渠道下发的视频字节按内容哈希落到 multimedia/，返回可登记的读取地址
func saveVideoFileURL(name string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if base == "" || base == "." || base == "/" {
		base = "视频.mp4"
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" || resource.KindOfExt(ext) != resource.KindVideo {
		ext = ".mp4"
	}
	sum := sha256.Sum256(data)
	rel := fmt.Sprintf("multimedia/video_%s%s", hex.EncodeToString(sum[:8]), ext)
	result, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		logger.SubWarn("LunarCore", "Napcat", "入站视频落盘失败（%s）: %v", base, err)
		return ""
	}
	// 内容仓库可能按哈希复用既有路径（例如 multimedia/33bfb...mp4）。
	// 必须使用实际落盘路径，否则会登记一个指向不存在文件的 @U 地址。
	if saved := strings.TrimSpace(result.RelPath); saved != "" {
		rel = saved
	}
	return config.LocalBaseURL() + channelFileReadPath + rel
}
