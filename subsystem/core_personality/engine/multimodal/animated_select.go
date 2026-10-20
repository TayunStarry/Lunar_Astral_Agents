package multimodal

// animated_select.go 动态图理解帧选择（数量骨架 · 回退路径）
//
// 本文件只保留不依赖帧内容的纯数量骨架：分析通道（animated_analyze.go /
// animated_smart.go）不可用时的兜底。策略为「开头稠密、结尾稀疏」的加权分组：
//
//	<= 64 帧     全部提交
//	65 ~ 128    16 帧一组，取 4 组
//	129 ~ 256    8 帧一组，取 8 组
//	257 ~ 512    4 帧一组，取 16 组
//	513 ~ 1024   2 帧一组，取 32 组
//	> 1024       1 帧一组，取 64 组
//
// 组的位置按「间隔递增」分布：开头相邻组间隔很小，越往后间隔越大，
// 即开头稠密、结尾稀疏，而不是把整个动画均匀切一遍导致哪里都没看透。

import (
	"math"
	"sort"
)

// AnimatedMaxUnderstandFrames 单次动态图理解提交给模型的帧数上限
//
// 64 帧的依据（Gemma-4 12B 实测，640 长边、默认视觉预算）：
// 5:3 单帧约 106 token，1:1 单帧约 171 token；64 帧合计约 6.8k~10.9k token，
// 叠加感知者提示词与输出后仍能落在 20k 上下文内。
const AnimatedMaxUnderstandFrames = 64

// SelectAnimatedFrameIndices 从 n 帧中挑选用于理解的帧下标（升序、去重、<= AnimatedMaxUnderstandFrames）
func SelectAnimatedFrameIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	if n <= AnimatedMaxUnderstandFrames {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}

	groupSize, groupCount := 16, 4
	switch {
	case n > 1024:
		groupSize, groupCount = 1, 64
	case n > 512:
		groupSize, groupCount = 2, 32
	case n > 256:
		groupSize, groupCount = 4, 16
	case n > 128:
		groupSize, groupCount = 8, 8
	}

	starts := animatedGroupStarts(n, groupSize, groupCount)
	out := make([]int, 0, AnimatedMaxUnderstandFrames)
	seen := make(map[int]bool, AnimatedMaxUnderstandFrames)
	for _, start := range starts {
		end := start + groupSize
		if end > n {
			end = n
		}
		for i := start; i < end && len(out) < AnimatedMaxUnderstandFrames; i++ {
			if seen[i] {
				continue
			}
			seen[i] = true
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}

// animatedGroupStarts 计算每组连续帧的起始下标
//
// 组间权重取 1,2,3,...（间隔递增）：第一组固定在 0，最后一组收在末帧，
// 中间各组按累计权重落位，因此开头密集、越往后越稀疏。
//
// 加权落位后还要保证相邻组不重叠：总帧数刚超过上限时（如 65 帧取 4×16），
// 纯加权会把第 2 组压到第 1 组上，去重后不足上限帧数。
func animatedGroupStarts(n, groupSize, groupCount int) []int {
	if groupCount <= 1 {
		return []int{0}
	}
	maxStart := n - groupSize
	if maxStart <= 0 {
		return []int{0}
	}
	starts := make([]int, groupCount)
	starts[0] = 0
	totalWeight := float64((groupCount - 1) * groupCount / 2)
	acc := 0.0
	for i := 1; i < groupCount; i++ {
		acc += float64(i)
		starts[i] = int(math.Round(float64(maxStart) * acc / totalWeight))
	}
	starts[groupCount-1] = maxStart
	// 从左到右拉开重叠的组
	for i := 1; i < groupCount; i++ {
		if minStart := starts[i-1] + groupSize; starts[i] < minStart {
			starts[i] = minStart
		}
	}
	// 从右到左回收越界：末组必须收在末帧，前面的组依次左移
	starts[groupCount-1] = maxStart
	for i := groupCount - 2; i >= 0; i-- {
		if maxStartOfGroup := starts[i+1] - groupSize; starts[i] > maxStartOfGroup {
			starts[i] = maxStartOfGroup
		}
	}
	return starts
}

// SelectAnimatedFrames 按下标选择规则筛选已解码的帧序列
func SelectAnimatedFrames(frames []MediaFrame) []MediaFrame {
	indices := SelectAnimatedFrameIndices(len(frames))
	out := make([]MediaFrame, 0, len(indices))
	for _, index := range indices {
		out = append(out, frames[index])
	}
	return out
}
