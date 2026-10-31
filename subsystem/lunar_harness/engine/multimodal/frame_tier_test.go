package multimodal

// frame_tier_test.go 自适应定档的纯函数回归测试
//
// 钉住四条规则：
//  1. 运动强度四档阈值与时长下限抬升互不干扰
//  2. 120 秒以上安静切片至少 32 帧，300 秒以上直接 48 帧
//  3. 非法/零时长切片不回退成 0 帧，仍给常规档
//  4. 定档结果恒落在 [WatchFramesMin, WatchFramesMax] 区间

import "testing"

// TestFrameTierByMotion 运动强度四档：静止 16 / 常规 24 / 快节奏 32 / 剧烈 48
func TestFrameTierByMotion(t *testing.T) {
	cases := []struct {
		name   string
		span   float64
		motion float64
		want   int
	}{
		{"几乎静止", 60, motionQuiet - 0.1, WatchFramesMin},
		{"常规运动", 60, motionCalm - 0.1, 24},
		{"快节奏", 60, motionBusy - 0.1, 32},
		{"剧烈变化", 60, motionBusy + 5, WatchFramesMax},
	}
	for _, c := range cases {
		if got := frameTierFor(c.span, c.motion); got != c.want {
			t.Fatalf("%s（span=%.0f motion=%.1f）应为 %d 帧，实得 %d", c.name, c.span, c.motion, c.want, got)
		}
	}
}

// TestFrameTierRaiseBySpan 长切片在低运动档位上的下限抬升
func TestFrameTierRaiseBySpan(t *testing.T) {
	if got := frameTierFor(longSegmentSeconds+10, 0); got != 32 {
		t.Fatalf("120 秒以上的静止切片应抬到 32 帧，实得 %d", got)
	}
	if got := frameTierFor(veryLongSegmentSeconds+10, 0); got != WatchFramesMax {
		t.Fatalf("300 秒以上的静止切片应直接取 48 帧，实得 %d", got)
	}
	// 抬升只补下限，不压低本来更高的档位
	if got := frameTierFor(veryLongSegmentSeconds+10, motionBusy+5); got != WatchFramesMax {
		t.Fatalf("超长剧烈切片仍应为 48 帧，实得 %d", got)
	}
}

// TestFrameTierBounds 任何输入下都落在合法帧数区间
func TestFrameTierBounds(t *testing.T) {
	for _, span := range []float64{0, -1, 1, 30, 61, 121, 301, 3600} {
		for _, motion := range []float64{-5, 0, 2.9, 3, 9.9, 10, 21.9, 22, 100} {
			got := frameTierFor(span, motion)
			if got < WatchFramesMin || got > WatchFramesMax {
				t.Fatalf("span=%.0f motion=%.1f 定档越界: %d", span, motion, got)
			}
		}
	}
}
