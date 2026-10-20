package multimodal

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os/exec"
	"testing"
)

// TestAnimatedImageToFramesSmoke 真实走一遍观察者链路：GIF → ffprobe 帧数 → select 选帧
func TestAnimatedImageToFramesSmoke(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("未安装 ffmpeg，跳过动态图抽帧冒烟测试")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("未安装 ffprobe，跳过动态图抽帧冒烟测试")
	}

	var buf bytes.Buffer
	palette := color.Palette{color.Black, color.White, color.RGBA{R: 255, A: 255}}
	animation := &gif.GIF{LoopCount: 1}
	// 画布 100×64：条纹位置 0..79 全程在画面内且互不重复，
	// 避免条纹移出后尾帧退化为重复画面被感知去重收敛
	for i := 0; i < 80; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 100, 64), palette)
		for y := 0; y < 64; y++ {
			for x := 0; x < 100; x++ {
				if x >= i && x < i+16 {
					frame.SetColorIndex(x, y, 2)
				} else {
					frame.SetColorIndex(x, y, uint8(i%2))
				}
			}
		}
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 5)
	}
	animation.Config = image.Config{ColorModel: palette, Width: 100, Height: 64}
	if err := gif.EncodeAll(&buf, animation); err != nil {
		t.Fatalf("生成测试 GIF 失败: %v", err)
	}

	groups, err := AnimatedImageToFrames(buf.Bytes())
	if err != nil {
		t.Fatalf("动态图抽帧失败: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("应返回单个分组，实际 %d", len(groups))
	}
	frames := groups[0].Frames
	if len(frames) != AnimatedMaxUnderstandFrames {
		t.Fatalf("80 帧动态图应选择 %d 帧，实际 %d", AnimatedMaxUnderstandFrames, len(frames))
	}
	if frames[0].Image == "" || frames[len(frames)-1].Image == "" {
		t.Fatalf("帧数据为空")
	}
}

// TestSelectAnimatedFrameIndices 验证帧数上限、升序去重与「开头稠密」分布
func TestSelectAnimatedFrameIndices(t *testing.T) {
	cases := []int{1, 20, 32, 64, 65, 80, 100, 128, 129, 256, 257, 512, 513, 1024, 1025, 5000}
	for _, n := range cases {
		indices := SelectAnimatedFrameIndices(n)
		if len(indices) > AnimatedMaxUnderstandFrames {
			t.Fatalf("n=%d 选中 %d 帧，超过上限 %d", n, len(indices), AnimatedMaxUnderstandFrames)
		}
		if n <= AnimatedMaxUnderstandFrames && len(indices) != n {
			t.Fatalf("n=%d 应全部提交，实际 %d 帧", n, len(indices))
		}
		for i, index := range indices {
			if index < 0 || index >= n {
				t.Fatalf("n=%d 下标越界: %d", n, index)
			}
			if i > 0 && index <= indices[i-1] {
				t.Fatalf("n=%d 下标未严格升序: %v", n, indices)
			}
		}
		if n > AnimatedMaxUnderstandFrames && len(indices) != AnimatedMaxUnderstandFrames {
			t.Fatalf("n=%d 应选满 %d 帧，实际 %d", n, AnimatedMaxUnderstandFrames, len(indices))
		}
		// 首帧与末帧必须被覆盖（结尾组收在末帧）
		if n > AnimatedMaxUnderstandFrames {
			if indices[0] != 0 {
				t.Fatalf("n=%d 应从首帧开始，实际 %d", n, indices[0])
			}
			if indices[len(indices)-1] != n-1 {
				t.Fatalf("n=%d 应收在末帧，实际 %d", n, indices[len(indices)-1])
			}
		}
	}
}

// TestSelectAnimatedFramesPrefersEarly 长动画的选择应开头稠密、结尾稀疏
func TestSelectAnimatedFramesPrefersEarly(t *testing.T) {
	n := 5000
	indices := SelectAnimatedFrameIndices(n)
	firstTenth := 0
	lastTenth := 0
	for _, index := range indices {
		if index < n/10 {
			firstTenth++
		}
		if index >= n-n/10 {
			lastTenth++
		}
	}
	if firstTenth <= lastTenth {
		t.Fatalf("应开头稠密、结尾稀疏: 前 10%% 选中 %d，后 10%% 选中 %d", firstTenth, lastTenth)
	}
}

// TestSelectAnimatedFramesMapping 按下标映射到帧对象
func TestSelectAnimatedFramesMapping(t *testing.T) {
	frames := make([]MediaFrame, 100)
	for i := range frames {
		frames[i] = MediaFrame{Image: "frame", Time: float64(i)}
	}
	selected := SelectAnimatedFrames(frames)
	if len(selected) != AnimatedMaxUnderstandFrames {
		t.Fatalf("应选择 %d 帧，实际 %d", AnimatedMaxUnderstandFrames, len(selected))
	}
	if selected[0].Time != 0 || selected[len(selected)-1].Time != 99 {
		t.Fatalf("首末帧映射异常: %v ~ %v", selected[0].Time, selected[len(selected)-1].Time)
	}
}
