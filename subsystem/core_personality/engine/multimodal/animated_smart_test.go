package multimodal

// animated_smart_test.go 分层自适应选帧的回归测试
//
// 用纯色网格构造四种典型动画：静止、循环、匀速运动、长轴突变，
// 验证循环裁剪、均匀保底、运动窗口加密与感知去重四条规则。

import (
	"testing"
)

// makeAnalysis 由每帧灰度值构建分析产物（32×32 纯色网格）
func makeAnalysis(values []int) *animatedAnalysis {
	a := &animatedAnalysis{
		Grids:  make([][]byte, len(values)),
		Hashes: make([]uint64, len(values)),
		Diffs:  make([]float64, len(values)),
	}
	for i, v := range values {
		g := make([]byte, animatedGridSize)
		for j := range g {
			g[j] = byte(v)
		}
		a.Grids[i] = g
		a.Hashes[i] = averageHash(g)
		if i > 0 {
			a.Diffs[i] = gridDiff(g, a.Grids[i-1])
		}
	}
	return a
}

// containsInt 判断切片是否包含目标值
func containsInt(vals []int, target int) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}

// TestSelectSmartStatic 静止动画：循环裁剪 + 去重坍缩，仅保留最小样本
func TestSelectSmartStatic(t *testing.T) {
	values := make([]int, 200)
	a := makeAnalysis(values)
	indices := SelectSmartAnimatedFrames(a)
	if len(indices) != animatedMinKeep {
		t.Fatalf("静止动画应仅保留 %d 帧，实际 %d 帧: %v", animatedMinKeep, len(indices), indices)
	}
	for _, idx := range indices {
		if idx >= animatedLoopMinPeriod {
			t.Fatalf("静止动画经循环裁剪后下标应落在单周期内: %d", idx)
		}
	}
}

// TestSelectSmartLoop 循环动画：只保留一个周期内的帧
func TestSelectSmartLoop(t *testing.T) {
	values := make([]int, 120)
	for i := range values {
		values[i] = (i % 20) * 6
	}
	a := makeAnalysis(values)
	indices := SelectSmartAnimatedFrames(a)
	if len(indices) != 20 {
		t.Fatalf("周期 20 的循环动画应保留单周期 20 帧，实际 %d 帧: %v", len(indices), indices)
	}
	if indices[len(indices)-1] != 19 {
		t.Fatalf("单周期应收在末帧 19，实际 %d", indices[len(indices)-1])
	}
}

// TestSelectSmartUniform 中等长度：均匀 64 帧覆盖全时间轴，首尾锚点齐备
func TestSelectSmartUniform(t *testing.T) {
	values := make([]int, 80)
	for i := range values {
		values[i] = 20 + i*2
	}
	a := makeAnalysis(values)
	indices := SelectSmartAnimatedFrames(a)
	if len(indices) != AnimatedMaxUnderstandFrames {
		t.Fatalf("80 帧动画应均匀选取 64 帧，实际 %d 帧", len(indices))
	}
	if indices[0] != 0 || indices[len(indices)-1] != 79 {
		t.Fatalf("首尾锚点缺失: %d ~ %d", indices[0], indices[len(indices)-1])
	}
}

// TestSelectSmartBurst 长时间轴：均匀保底覆盖首尾，运动窗口锁定突变瞬间
func TestSelectSmartBurst(t *testing.T) {
	values := make([]int, 1000)
	for i := range values {
		v := 40 + i/10 // 缓慢漂移：每 10 帧 +1
		if i >= 300 && i < 360 {
			v += 120 // 中段突变：进入（300）与离开（360）各产生一次大幅差异
		}
		values[i] = v
	}
	a := makeAnalysis(values)
	indices := SelectSmartAnimatedFrames(a)
	if len(indices) > AnimatedMaxUnderstandFrames {
		t.Fatalf("选中 %d 帧，超过上限 %d", len(indices), AnimatedMaxUnderstandFrames)
	}
	if indices[0] != 0 || indices[len(indices)-1] != 999 {
		t.Fatalf("首尾锚点缺失: %d ~ %d", indices[0], indices[len(indices)-1])
	}
	if !containsInt(indices, 300) {
		t.Fatalf("运动窗口应覆盖突变进入点 300: %v", indices)
	}
	if !containsInt(indices, 360) {
		t.Fatalf("运动窗口应覆盖突变离开点 360: %v", indices)
	}
}
