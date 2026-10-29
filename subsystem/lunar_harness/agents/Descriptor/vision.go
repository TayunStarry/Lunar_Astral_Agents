package Descriptor

// vision.go 视觉读取副本：静态图片进入模型前的唯一合法形态
//
// 与 /file/miniature 端点、visionRead 共用 multimodal.VisionTransform：
// 缩放到 vision_max_side、右上角叠印句柄、按内容哈希落盘缓存。
// 原图不被修改，也不会直接交给模型。

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// resourceVisionURI 生成静态图片的视觉副本并返回 data URI
func resourceVisionURI(res resource.Resource, abs string) (string, error) {
	side := config.VisionMaxSide
	if side <= 0 {
		side = 1080
	}
	sourceURL := strings.TrimSpace(res.URL)
	if sourceURL == "" {
		sourceURL = resource.FileReadURL(res.BlobPath)
	}
	hash := strings.TrimSpace(res.ContentHash)
	if hash == "" {
		sum := sha256.Sum256([]byte(sourceURL))
		hash = hex.EncodeToString(sum[:8])
	}
	cacheRel := filepath.ToSlash(filepath.Join("multimedia", "vision", hash+"_s"+strconv.Itoa(side)+".jpg"))
	cacheAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(cacheRel))

	data, mime, cached, err := multimodal.VisionTransform(abs, cacheAbs, side, res.Handle)
	if err != nil {
		return "", fmt.Errorf("生成视觉副本失败: %w", err)
	}
	if cached {
		resource.VisionCacheTouch(sourceURL, side)
	} else {
		_ = resource.VisionCachePut(resource.VisionCacheEntry{
			SourceURL: sourceURL,
			Side:      side,
			Tag:       res.Handle,
			CachePath: cacheRel,
			Bytes:     int64(len(data)),
		})
		if removed, freed := resource.VisionCacheEvict(int64(config.VisionCacheCapMB) << 20); removed > 0 {
			logger.Info("CrystalAstral", "描述者：视觉副本缓存淘汰 %d 个（释放 %d 字节）", removed, freed)
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
