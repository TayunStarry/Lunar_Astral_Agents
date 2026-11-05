//go:build windows

package desktop

// resize_equiv_test.go 验证「engine/desktop 改用 engine/multimodal.ResizeToFit」前后行为等价。
//
// 改造前 desktop 内复刻了一份 ResizeToFit；现在统一调用 multimodal 的唯一实现。
// 本文件保留旧实现作为基线（仅测试使用，不参与生产逻辑），逐像素比对证明无行为漂移。

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"LunarSubsystem/LunarHarness/engine/multimodal"

	"github.com/disintegration/imaging"
)

// legacyResizeToFit 是改造前 engine/desktop 内的本地复刻实现（等价性基线）。
func legacyResizeToFit(img *image.RGBA, maxWidth, maxHeight int) *image.RGBA {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	if width <= maxWidth && height <= maxHeight {
		return img
	}
	wRatio := float64(maxWidth) / float64(width)
	hRatio := float64(maxHeight) / float64(height)
	scale := wRatio
	if hRatio < wRatio {
		scale = hRatio
	}
	width = int(float64(width)*scale + 0.5)
	height = int(float64(height)*scale + 0.5)
	if width > maxWidth {
		width = maxWidth
	}
	if height > maxHeight {
		height = maxHeight
	}
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	return legacyToRGBA(resized)
}

func legacyToRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba
}

// syntheticRGBA 生成确定性的渐变图，覆盖不同灰度与透明度。
func syntheticRGBA(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 11) % 256),
				B: uint8((x + y) % 256),
				A: uint8(128 + (x+y)%128),
			})
		}
	}
	return img
}

func TestResizeToFitMatchesLegacy(t *testing.T) {
	base := syntheticRGBA(320, 180)
	cases := []struct {
		name                string
		maxWidth, maxHeight int
	}{
		{"放大上限", 768, 768},
		{"缩小缩略图", 96, 96},
		{"等比受宽限制", 100, 50},
		{"等比受高限制", 50, 100},
		{"原尺寸不缩放", 320, 180},
		{"略小于原图", 300, 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := multimodal.ResizeToFit(base, tc.maxWidth, tc.maxHeight)
			want := legacyResizeToFit(base, tc.maxWidth, tc.maxHeight)

			gb, wb := got.Bounds(), want.Bounds()
			if gb.Dx() != wb.Dx() || gb.Dy() != wb.Dy() {
				t.Fatalf("尺寸不一致：multimodal=%dx%d legacy=%dx%d", gb.Dx(), gb.Dy(), wb.Dx(), wb.Dy())
			}
			for y := 0; y < gb.Dy(); y++ {
				for x := 0; x < gb.Dx(); x++ {
					g := got.RGBAAt(x, y)
					w := want.RGBAAt(x, y)
					if diffExceeds(g.R, w.R, 1) || diffExceeds(g.G, w.G, 1) ||
						diffExceeds(g.B, w.B, 1) || diffExceeds(g.A, w.A, 1) {
						t.Fatalf("像素 (%d,%d) 超出容差：multimodal=%v legacy=%v", x, y, g, w)
					}
				}
			}
		})
	}
}

func diffExceeds(a, b uint8, tolerance int) bool {
	diff := int(a) - int(b)
	if diff < 0 {
		diff = -diff
	}
	return diff > tolerance
}
