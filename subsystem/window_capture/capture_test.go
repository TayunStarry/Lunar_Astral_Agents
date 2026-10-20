package windowcapture

import (
	"image"
	"image/color"
	"testing"
)

func TestIsUniform(t *testing.T) {
	uniform := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(uniform.Pix); i += 4 {
		uniform.Pix[i], uniform.Pix[i+1], uniform.Pix[i+2], uniform.Pix[i+3] = 0, 0, 0, 255
	}
	if !IsUniform(uniform) {
		t.Fatal("纯黑图应判定为纯色")
	}

	nonUniform := image.NewRGBA(image.Rect(0, 0, 8, 8))
	copy(nonUniform.Pix, uniform.Pix)
	nonUniform.Set(0, 0, color.RGBA{R: 255, A: 255})
	if IsUniform(nonUniform) {
		t.Fatal("含不同像素的图不应判定为纯色")
	}

	if !IsUniform(nil) {
		t.Fatal("nil 图应判定为不可用")
	}
}
