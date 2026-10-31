package LunarPersonality

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
)

var (
	visionTagPattern = regexp.MustCompile(`^@[A-Z]\.\d{3,}$`)
	visionImageExts  = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
	}
)

// visionReadBlob 取思维链消费副本：缩放到约束尺寸、叠印句柄、按内容寻址缓存
func visionReadBlob(relPath, tag string, side int) ([]byte, string, error) {
	if side <= 0 {
		side = config.VisionMaxSide
	}
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return nil, "", fmt.Errorf("非法路径: %s", relPath)
	}
	if !strings.HasPrefix(clean, "multimedia/") {
		return nil, "", fmt.Errorf("视觉读取仅允许 multimedia/ 下的文件: %s", relPath)
	}
	if !visionImageExts[strings.ToLower(filepath.Ext(clean))] {
		return nil, "", fmt.Errorf("视觉读取仅允许图片类文件: %s", relPath)
	}
	if !visionTagPattern.MatchString(tag) {
		tag = ""
	}
	if err := resource.EnsureReady(); err != nil {
		return nil, "", fmt.Errorf("资源目录初始化失败: %w", err)
	}
	contentHash := ""
	sourceURL := resource.FileReadURL(clean)
	if entry, ok := resource.ForPath(clean); ok {
		if tag == "" {
			tag = entry.Handle
		}
		if strings.TrimSpace(entry.URL) != "" {
			sourceURL = entry.URL
		}
		contentHash = entry.ContentHash
	}
	if contentHash == "" {
		sum := sha256.Sum256([]byte(sourceURL))
		contentHash = hex.EncodeToString(sum[:8])
	}
	cacheRel := "multimedia/vision/" + contentHash + "_s" + strconv.Itoa(side) + ".jpg"
	data, mime, cached, err := multimodal.VisionTransform(
		filepath.Join(config.LocalDir, clean),
		filepath.Join(config.LocalDir, cacheRel),
		side, tag,
	)
	if err != nil {
		return nil, "", err
	}
	if cached {
		resource.VisionCacheTouch(sourceURL, side)
		return data, mime, nil
	}
	_ = resource.VisionCachePut(resource.VisionCacheEntry{
		SourceURL: sourceURL,
		Side:      side,
		Tag:       tag,
		CachePath: cacheRel,
		Bytes:     int64(len(data)),
	})
	_, _ = resource.VisionCacheEvict(int64(config.VisionCacheCapMB) << 20)
	return data, mime, nil
}

// VisionReadBlob 对外暴露视觉读取副本（HTTP 端点使用）
func VisionReadBlob(relPath, tag string, side int) ([]byte, string, error) {
	return visionReadBlob(relPath, tag, side)
}

// visionCopy 取副本并返回像素尺寸
func visionCopy(relPath, tag string, side int) (data []byte, mime string, width, height int, err error) {
	data, mime, err = visionReadBlob(relPath, tag, side)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
		width, height = cfg.Width, cfg.Height
	}
	return data, mime, width, height, nil
}

// visionReference 生成图片的思维链消费引用
func visionReference(handle, blobPath string) string {
	if config.ImageInputMode == "base64" {
		if data, mime, _, _, err := visionCopy(blobPath, handle, 0); err == nil {
			return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	return config.LocalBaseURL() + visionReadPath + blobPath + "?tag=" + url.QueryEscape(handle)
}

// resolveImageBytes 把来源地址解析为图片字节（支持 data URI、/file/read/、multimedia/ 相对路径与本地文件）
func resolveImageBytes(source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("图片数据为空")
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "data:") {
		marker := strings.Index(source, "base64,")
		if marker < 0 {
			return nil, fmt.Errorf("不支持的 data URI")
		}
		return base64.StdEncoding.DecodeString(source[marker+len("base64,"):])
	}
	rel := source
	for _, prefix := range []string{"/file/read/", "/file/miniature/"} {
		if strings.HasPrefix(rel, prefix) {
			rel = strings.TrimPrefix(rel, prefix)
			break
		}
	}
	if cut := strings.IndexAny(rel, "?#"); cut >= 0 {
		rel = rel[:cut]
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		// 腾讯图床只接受 https：以 http 直链 GET 会被边缘节点直接拒绝（400）。
		source = resource.UpgradeCDNScheme(source)
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("图片地址返回 %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(config.LocalDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
