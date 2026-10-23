package multimodal

// animated_analyze.go 动态图逐帧分析通道
//
// 在正式抽帧前，用 ffmpeg 把全部帧一次性缩略为 32×32 灰度网格（rawvideo 管道输出，
// 每帧恰好 animatedGridSize 字节），在内存中派生三类信号供智能选帧使用：
//   - Grids  缩略网格：近重复判定的复核依据
//   - Hashes 8×8 平均哈希：近重复与循环周期的快速初筛
//   - Diffs  相邻帧平均差（0-1）：运动强度，驱动运动窗口加密
//
// 通道不落盘、不改动原有解码链路；失败时调用方回退为纯数量骨架选帧。

import (
	"bytes"
	"fmt"
	"math/bits"
	"os"
	"strconv"

	"LunarSubsystem/GeneralConfig"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// animatedAnalyzeCap 逐帧分析的最大帧数（极端长动画截断，与历史安全上限一致）
const animatedAnalyzeCap = 2048

// 缩略网格规格
const (
	animatedGridDim  = 32
	animatedGridSize = animatedGridDim * animatedGridDim
)

// animatedAnalysis 动态图逐帧分析产物
type animatedAnalysis struct {
	// Grids 每帧的 32×32 灰度网格（行优先，每帧长度 animatedGridSize）
	Grids [][]byte
	// Hashes 每帧的 8×8 平均感知哈希
	Hashes []uint64
	// Diffs 相邻帧平均差（0-1 标准化；Diffs[0] 恒为 0）
	Diffs []float64
}

// analyzeAnimatedFrames 解码全部帧的缩略网格并计算差异与哈希
func analyzeAnimatedFrames(inputFile string, maxFrames int) (*animatedAnalysis, error) {
	if maxFrames <= 0 || maxFrames > animatedAnalyzeCap {
		maxFrames = animatedAnalyzeCap
	}
	buf := &bytes.Buffer{}
	stream := ffmpeg.Input(inputFile).Output("pipe:1", ffmpeg.KwArgs{
		"vf":       fmt.Sprintf("scale=%d:%d", animatedGridDim, animatedGridDim),
		"pix_fmt":  "gray",
		"f":        "rawvideo",
		"vsync":    "0",
		"frames:v": strconv.Itoa(maxFrames),
		"loglevel": "error",
	})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	if err := stream.WithOutput(buf, os.Stderr).Run(); err != nil {
		return nil, fmt.Errorf("缩略解码失败: %w", err)
	}
	data := buf.Bytes()
	if len(data) < animatedGridSize {
		return nil, fmt.Errorf("缩略数据不足: %d bytes", len(data))
	}
	count := len(data) / animatedGridSize
	a := &animatedAnalysis{
		Grids:  make([][]byte, count),
		Hashes: make([]uint64, count),
		Diffs:  make([]float64, count),
	}
	for i := 0; i < count; i++ {
		g := data[i*animatedGridSize : (i+1)*animatedGridSize]
		a.Grids[i] = g
		a.Hashes[i] = averageHash(g)
		if i > 0 {
			a.Diffs[i] = gridDiff(g, a.Grids[i-1])
		}
	}
	return a, nil
}

// gridDiff 两幅缩略网格的平均绝对差（0-1 标准化；长度不一致视为完全不同）
func gridDiff(a, b []byte) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 1
	}
	sum := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return float64(sum) / float64(len(a)) / 255
}

// averageHash 计算 8×8 平均感知哈希：32×32 网格按 4×4 块降采样后与块均值比较
func averageHash(grid []byte) uint64 {
	const blocks = 8
	var cell [blocks * blocks]int
	for y := 0; y < animatedGridDim; y++ {
		for x := 0; x < animatedGridDim; x++ {
			cell[(y/4)*blocks+(x/4)] += int(grid[y*animatedGridDim+x])
		}
	}
	mean := 0
	for _, v := range cell {
		mean += v
	}
	mean /= len(cell)
	var h uint64
	for i, v := range cell {
		if v > mean {
			h |= 1 << uint(i)
		}
	}
	return h
}

// hashDistance 两个感知哈希的汉明距离
func hashDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
