package napcat

// image_media.go QQ 图片出站压缩
//
// QQ 的富媒体上传对单张图片的体积 / 尺寸有隐性上限：超过后 sendMsg 会返回
// 「rich media transfer failed」。典型场景是绘图作品开了 4x 超分（例如
// 6144x4096、38MB 的 PNG），宿主的 /file/read/ 下载已经成功，卡在 QQ 上传。
//
// 这里在交给 NapCat 前把「本机内容仓库里的超大静态图」压成 2048 长边的 JPEG；
// 原图保持不动（前端仍看原图），只影响 QQ 出站副本。GIF / WebP 跳过，避免把
// 动图压成单帧。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

const (
	// qqImageMaxDimension 原始长边超过该值时压缩（QQ 对大尺寸图上传不稳定）
	qqImageMaxDimension = 4096
	// qqImageMaxBytes 原始体积超过该值时压缩
	qqImageMaxBytes = 10 << 20
	// qqImageTargetSide 压缩后的长边（2048 足够群聊查看，且远低于 QQ 上限）
	qqImageTargetSide = 2048
	// qqImageJpegQuality 压缩副本的 JPEG 质量
	qqImageJpegQuality = 90
)

// compressImageForQQ 把超出 QQ 上传上限的本机图片压成临时 JPEG。
//
// 返回临时文件绝对路径；不需要压缩 / 不是本机内容仓库图片 / 解码失败时返回 "", false，
// 调用方沿用原地址。调用方负责在发送完成后删除临时文件。
func compressImageForQQ(img string) (string, bool) {
	path, ok := localContentPath(img)
	if !ok {
		return "", false
	}
	// 动图保持原样：压成单帧会丢掉动画语义
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".gif" || ext == ".webp" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	cfg, _, err := image.DecodeConfig(file)
	file.Close()
	if err != nil {
		return "", false
	}
	if info.Size() <= qqImageMaxBytes && cfg.Width <= qqImageMaxDimension && cfg.Height <= qqImageMaxDimension {
		return "", false
	}

	file, err = os.Open(path)
	if err != nil {
		return "", false
	}
	src, _, err := image.Decode(file)
	file.Close()
	if err != nil {
		LoggerGeneral.SubWarn("LunarCore", "Napcat", "QQ 图片压缩解码失败（%s）: %v", filepath.Base(path), err)
		return "", false
	}

	// 等比缩放到长边 2048（imaging 传 0 表示按比例自动计算另一边）
	var resized *image.NRGBA
	if src.Bounds().Dx() >= src.Bounds().Dy() {
		resized = imaging.Resize(src, qqImageTargetSide, 0, imaging.Lanczos)
	} else {
		resized = imaging.Resize(src, 0, qqImageTargetSide, imaging.Lanczos)
	}

	// JPEG 无透明通道：先铺白底再合成，避免透明区域变黑
	canvas := image.NewRGBA(resized.Bounds())
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), resized, resized.Bounds().Min, draw.Over)

	tmp, err := os.CreateTemp("", "lunar-qq-image-*.jpg")
	if err != nil {
		LoggerGeneral.SubWarn("LunarCore", "Napcat", "QQ 图片压缩创建临时文件失败: %v", err)
		return "", false
	}
	if err := jpeg.Encode(tmp, canvas, &jpeg.Options{Quality: qqImageJpegQuality}); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		LoggerGeneral.SubWarn("LunarCore", "Napcat", "QQ 图片压缩编码失败: %v", err)
		return "", false
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", false
	}

	LoggerGeneral.SubInfo("LunarCore", "Napcat",
		"QQ 图片超出上传上限，已压缩: %s %dx%d %.1fMB → %dx%d JPEG",
		filepath.Base(path), cfg.Width, cfg.Height, float64(info.Size())/(1<<20),
		resized.Bounds().Dx(), resized.Bounds().Dy())
	return tmp.Name(), true
}

// localContentPath 把 /file/read/<rel> 或裸内容仓库路径解析为 LocalDir 内的绝对路径。
// 只接受 LocalDir 内的文件；远程 URL、base64 等返回 false（不参与压缩）。
func localContentPath(img string) (string, bool) {
	raw := strings.TrimSpace(img)
	if raw == "" {
		return "", false
	}
	// 视觉副本等地址可能带查询串/锚点，文件系统路径不含它们
	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		raw = raw[:cut]
	}
	// 本机 HTTP 地址：还原成 /file/read/ 相对地址后再按内容仓库路径处理
	if idx := strings.Index(raw, "/file/read/"); idx >= 0 {
		raw = raw[idx:]
	}

	rel := ""
	switch {
	case strings.HasPrefix(raw, "/file/read/"):
		rel = strings.TrimPrefix(raw, "/file/read/")
	case strings.HasPrefix(raw, "multimedia/"), strings.HasPrefix(raw, "audios/"):
		rel = raw
	default:
		if !filepath.IsAbs(raw) {
			return "", false
		}
		return ensureUnderLocalDir(raw)
	}
	if decoded, err := url.PathUnescape(rel); err == nil {
		rel = decoded
	}
	return ensureUnderLocalDir(filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(rel)))
}

// ensureUnderLocalDir 确认路径位于 LocalDir 内（防止把任意本机文件读给 QQ）
func ensureUnderLocalDir(abs string) (string, bool) {
	base := filepath.Clean(*GeneralConfig.LocalDir)
	clean := filepath.Clean(abs)
	if !strings.HasPrefix(clean, base+string(os.PathSeparator)) {
		return "", false
	}
	return clean, true
}
