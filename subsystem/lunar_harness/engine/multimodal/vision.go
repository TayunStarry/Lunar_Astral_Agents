package multimodal

// vision.go 视觉读取变换：思维链统一消费的「约束尺寸 + 句柄叠印」副本生成
//
// 设计要点（docs/adr/0001「图片标签机制收敛」）：
//   - 原图永远不动：缩放与叠印只作用于派生副本，参考图（图生图基座）始终读原图 blob，
//     扩散引擎拉到的参考图上没有标签
//   - 副本按「内容哈希 + 边长」盘上缓存，内容寻址天然不可变，无失效问题
//   - 动图直通：多帧动图原样返回（不缩放、不叠印、不缓存）——
//     缩放会把动图坍缩成单帧静态图，下游 isAnimatedImage 判定将永远失效

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// VisionTransform 把一张本地图片变换为思维链消费副本：长边不超过 side，右上角叠印句柄标签。
//
// @param inAbsPath   源图绝对路径（原图，只读不写）
// @param cacheAbsPath 派生副本落盘位置（已存在时直接读回，跳过变换）
// @param side        副本最大边长（像素，只缩不放）
// @param tag         叠印到右上角的句柄文本（如 @I.003；空串表示不叠印）
// @return 副本字节、contentType（动图直通时为原格式；静态图统一为 image/jpeg）、是否命中缓存
func VisionTransform(inAbsPath, cacheAbsPath string, side int, tag string) ([]byte, string, bool, error) {
	// 读取原图字节（同时用于动图判定与直通返回）
	raw, err := os.ReadFile(inAbsPath)
	if err != nil {
		return nil, "", false, fmt.Errorf("读取源图失败: %w", err)
	}
	// 动图直通：不缩放、不叠印、不缓存，保证下游动图理解链路仍能识别与处理
	if IsAnimatedImage(raw) {
		return raw, animatedMIME(raw), false, nil
	}
	// 缓存命中：副本按内容寻址、天然不可变，直接读回
	if cached, readErr := os.ReadFile(cacheAbsPath); readErr == nil && len(cached) > 0 {
		return cached, "image/jpeg", true, nil
	}
	// 解码（EXIF 方向自动校正）
	src, err := imaging.Open(inAbsPath, imaging.AutoOrientation(true))
	if err != nil {
		return nil, "", false, fmt.Errorf("解码源图失败: %w", err)
	}
	// 只缩不放：长边/短边均不超过 side
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width > side || height > side {
		scale := math.Min(float64(side)/float64(width), float64(side)/float64(height))
		src = imaging.Resize(src, int(float64(width)*scale), int(float64(height)*scale), imaging.Lanczos)
	}
	// 统一转为可写像素格式（JPEG 解码产物 YCbCr 不支持 Set，无法直接绘制）
	work := imaging.Clone(src)
	if tag != "" {
		overlayVisionTag(work, tag)
	}
	// 编码 JPEG 副本（视觉模型消费件，体积优先；透明通道按黑底压平，可接受）
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, work, &jpeg.Options{Quality: config.JPEGQuality}); err != nil {
		return nil, "", false, fmt.Errorf("编码副本失败: %w", err)
	}
	// 落盘缓存（失败不阻断返回，下次请求重建）
	if mkErr := os.MkdirAll(filepath.Dir(cacheAbsPath), 0o755); mkErr == nil {
		if writeErr := os.WriteFile(cacheAbsPath, buf.Bytes(), 0o644); writeErr != nil {
			logger.SubInfo(LogModuleMedia, "Vision", "副本缓存写入失败（不影响本次返回）: %v", writeErr)
		}
	}
	return buf.Bytes(), "image/jpeg", false, nil
}

// overlayVisionTag 在图片右上角叠印句柄标签：半透明黑底药丸 + 白色 ASCII 文本
//
// 文本尺寸随图片大小缩放：basicfont 字模基准 7×13，按整数倍最近邻放大
// （以图片短边为基准，约每 360px 放大 1 倍，1080 级副本为 3×）——
// 视觉模型对图片有下采样，字模若只占画面高度 1% 会被采到读不出来，
// 药丸必须稳定占到短边的 4% 上下模型才能可靠 OCR。小图上不小于原始字模。
// 句柄（@I.003）为纯 ASCII，无需携带 TTF。连续多图场景下标签随图走，
// 消除「图片与相邻标签文本错配」的可能。
func overlayVisionTag(img *image.NRGBA, tag string) {
	const (
		charW = 7 // basicfont.Face7x13 单字符前进宽度
		charH = 13
	)
	// 缩放倍率：短边每 360px 计 1 倍，夹在 1~6 之间（1080 → 3×，540 → 2×）
	minSide := img.Bounds().Dx()
	if h := img.Bounds().Dy(); h < minSide {
		minSide = h
	}
	scale := (minSide + 359) / 360
	if scale < 1 {
		scale = 1
	} else if scale > 6 {
		scale = 6
	}
	// 白色文本条（透明底）
	strip := image.NewNRGBA(image.Rect(0, 0, len(tag)*charW, charH))
	drawer := &font.Drawer{
		Dst:  strip,
		Src:  image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(0, charH-3),
	}
	drawer.DrawString(tag)
	// 最近邻整数倍放大，保持字形边缘锐利
	scaled := imaging.Resize(strip, len(tag)*charW*scale, charH*scale, imaging.NearestNeighbor)
	// 药丸几何：内边距与外边距同步缩放
	padX, padY, margin := 3*scale, 2*scale, 4*scale
	pillW := scaled.Bounds().Dx() + padX*2
	pillH := scaled.Bounds().Dy() + padY*2
	x0 := img.Bounds().Dx() - pillW - margin
	y0 := margin
	if x0 < 0 || y0 < 0 {
		return // 图太小放不下药丸：放弃叠印，保留无标签副本
	}
	// 半透明黑底药丸
	draw.Draw(img, image.Rect(x0, y0, x0+pillW, y0+pillH), image.NewUniform(color.RGBA{R: 0, G: 0, B: 0, A: 165}), image.Point{}, draw.Over)
	// 叠放白色文本
	draw.Draw(img, image.Rect(x0+padX, y0+padY, x0+padX+scaled.Bounds().Dx(), y0+padY+scaled.Bounds().Dy()), scaled, image.Point{}, draw.Over)
}

// animatedMIME 按文件头推断动图 MIME（GIF / APNG / 动态 WebP）
func animatedMIME(raw []byte) string {
	switch {
	case len(raw) >= 6 && (bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a"))):
		return "image/gif"
	case len(raw) >= 12 && bytes.HasPrefix(raw, []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return "image/png"
	}
}
