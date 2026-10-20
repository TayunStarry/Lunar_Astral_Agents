package multimodal

// animated_smart.go 动态图分层自适应选帧（内容感知）
//
// 依赖分析通道（animated_analyze.go）提供的缩略网格、感知哈希与帧间差异：
//   1. 循环裁剪：检测循环周期，时间轴只保留一个周期（重复内容不重复送审）；
//   2. 分层取帧：单周期 <=64 帧全部提交；<=128 帧均匀取 64 帧；更长时
//      「42 帧均匀保底 + 3 个高变化窗口加密 22 帧」，首尾锚点由均匀骨架携带；
//   3. 感知去重：近乎相同的画面只保留首现帧，静止动画自动坍缩为极少数样本。
//
// 64 帧配额模板（长动画）：
//	首帧 1 + 尾帧 1（均匀骨架端点）
//	全局均匀 40 帧（均匀骨架内部）
//	剩余 22 帧 → 高变化窗口加密（8 + 7 + 7）
//	检测到循环 → 只保留一个周期内的关键帧

import (
	"sort"
)

// 智能选帧的配额与阈值
const (
	// animatedUniformFrames 长动画的全局均匀保底帧数（含首尾锚点）
	animatedUniformFrames = 42
	// animatedWindowCount 运动加密窗口数（配额 22 = 最高分窗口 8 + 其余 7×2）
	animatedWindowCount = 3
	// animatedWindowTopFrames 最高分窗口的加密帧数
	animatedWindowTopFrames = 8
	// animatedWindowFrames 其余窗口的加密帧数
	animatedWindowFrames = 7
	// animatedBucketCount 运动分桶数（窗口在桶内挑选）
	animatedBucketCount = 16
	// animatedLayeredMin 时间轴超过该帧数才启用运动窗口加密（更短时均匀 64 已足够稠密）
	animatedLayeredMin = 2 * AnimatedMaxUnderstandFrames
	// animatedLoopMinPeriod 循环周期的最短判定长度（更短的循环交给去重收敛）
	animatedLoopMinPeriod = 4
	// animatedDupHashDist 近重复判定的感知哈希汉明距离阈值（初筛）
	animatedDupHashDist = 3
	// animatedDupGridDiff 近重复判定的缩略网格平均差阈值（0-1 标准化，复核）
	animatedDupGridDiff = 0.006
	// animatedMinKeep 去重后至少保留的帧数（高度静止/循环的动画也给出可理解的最小样本）
	animatedMinKeep = 4
)

// SelectSmartAnimatedFrames 基于逐帧分析的分层自适应选帧（升序、去重、<= 64）
func SelectSmartAnimatedFrames(a *animatedAnalysis) []int {
	m := len(a.Grids)
	if m == 0 {
		return nil
	}
	end := animatedLoopEnd(a)
	var candidates []int
	switch {
	case end <= AnimatedMaxUnderstandFrames:
		// 单周期本就不长：全部提交，交由去重收缩重复画面
		candidates = evenIndices(end, end)
	case end <= animatedLayeredMin:
		// 中等长度：均匀 64 帧已足够稠密，无需窗口加密
		candidates = evenIndices(AnimatedMaxUnderstandFrames, end)
	default:
		// 长时间轴：均匀保底 + 运动窗口加密
		candidates = evenIndices(animatedUniformFrames, end)
		candidates = append(candidates, animatedWindowPicks(a, end)...)
	}
	sort.Ints(candidates)
	return animatedDedup(a, dedupSortedInts(candidates))
}

// animatedLoopEnd 检测循环周期，返回单周期覆盖的帧数（无循环返回全部帧数）
//
// 判定三重校验，全部通过才认定为循环：
//   - 起点对齐：第 p/p+1 帧分别与第 0/1 帧近重复；
//   - 中点对齐：时间轴中点落在周期对应相位上；
//   - 收尾对齐：末帧落回周期对应相位（真循环首尾衔接，漂移内容则对不上）。
//
// 仅查起点会被「起始静止段」误判成短周期（静止段内任意两帧都近似），
// 中点与收尾两查保证周期必须解释整条时间轴。
func animatedLoopEnd(a *animatedAnalysis) int {
	m := len(a.Grids)
	for p := animatedLoopMinPeriod; p+1 < m; p++ {
		if !animatedLooksSame(a, p, 0) || !animatedLooksSame(a, p+1, 1) {
			continue
		}
		if !animatedLooksSame(a, (m-1)/2, ((m-1)/2)%p) {
			continue
		}
		if animatedLooksSame(a, m-1, (m-1)%p) {
			return p
		}
	}
	return m
}

// animatedLooksSame 判断两帧是否近重复：感知哈希初筛 + 缩略网格复核
func animatedLooksSame(a *animatedAnalysis, i, j int) bool {
	if i < 0 || j < 0 || i >= len(a.Grids) || j >= len(a.Grids) {
		return false
	}
	if hashDistance(a.Hashes[i], a.Hashes[j]) > animatedDupHashDist {
		return false
	}
	return gridDiff(a.Grids[i], a.Grids[j]) < animatedDupGridDiff
}

// animatedWindowPicks 按帧间差异把 [0, end) 分桶，返回高变化窗口的加密帧下标
//
// 桶得分为桶内相邻帧差异之和；取得分最高的 3 个桶，相邻桶合并为同一窗口
// （一段连续动作常横跨多桶，如「进入 + 停留 + 离开」），每桶等距取帧并
// 强制包含桶内差异峰值帧。最高分桶多拿 1 帧配额（8 帧，其余 7 帧）。
func animatedWindowPicks(a *animatedAnalysis, end int) []int {
	buckets := animatedBucketCount
	if buckets > end {
		buckets = end
	}
	scores := make([]float64, buckets)
	for i := 1; i < end && i < len(a.Diffs); i++ {
		b := i * buckets / end
		if b >= buckets {
			b = buckets - 1
		}
		scores[b] += a.Diffs[i]
	}
	order := make([]int, buckets)
	for i := range order {
		order[i] = i
	}
	// 稳定排序：得分相同时保持桶序，保证选帧结果确定
	sort.SliceStable(order, func(x, y int) bool { return scores[order[x]] > scores[order[y]] })
	if len(order) > animatedWindowCount {
		order = order[:animatedWindowCount]
	}
	topBucket := order[0] // 降序首元素 = 全局最高分桶
	sort.Ints(order)

	out := make([]int, 0, animatedWindowCount*animatedWindowTopFrames)
	for _, b := range order {
		s, e := b*end/buckets, (b+1)*end/buckets
		k := animatedWindowFrames
		if b == topBucket {
			k = animatedWindowTopFrames
		}
		out = append(out, animatedWindowSample(a, s, e, k)...)
	}
	return out
}

// animatedWindowSample 在窗口 [s, e) 内取 k 帧：等距骨架 + 桶内差异峰值帧必选
func animatedWindowSample(a *animatedAnalysis, s, e, k int) []int {
	if e > len(a.Grids) {
		e = len(a.Grids)
	}
	if s < 0 || s >= e {
		return nil
	}
	if k > e-s {
		k = e - s
	}
	// 峰值帧：窗口内与前一帧差异最大处（动作瞬间）
	peak := s
	for i := s + 1; i < e; i++ {
		if a.Diffs[i] > a.Diffs[peak] {
			peak = i
		}
	}
	out := evenIndices(k, e-s)
	for i := range out {
		out[i] += s
	}
	for _, v := range out {
		if v == peak {
			return out
		}
	}
	// 峰值不在骨架中：替换中位帧，保持配额不变
	if len(out) > 0 {
		out[len(out)/2] = peak
	}
	return out
}

// animatedDedup 感知去重：按时间顺序保留首现帧，近重复画面直接丢弃
//
// 丢弃后不足 animatedMinKeep 帧（高度静止/循环）时，从候选中等距回补，
// 保证模型至少拿到可辨识的最小样本。
func animatedDedup(a *animatedAnalysis, candidates []int) []int {
	kept := make([]int, 0, len(candidates))
	for _, idx := range candidates {
		dup := false
		for _, k := range kept {
			if animatedLooksSame(a, idx, k) {
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, idx)
		}
	}
	if len(kept) < animatedMinKeep {
		for _, pi := range evenIndices(animatedMinKeep, len(candidates)) {
			cand := candidates[pi]
			found := false
			for _, k := range kept {
				if k == cand {
					found = true
					break
				}
			}
			if !found {
				kept = append(kept, cand)
			}
		}
		sort.Ints(kept)
	}
	return kept
}

// evenIndices 在 [0, end) 上等距取 count 个下标（恒含 0 与 end-1，升序去重）
func evenIndices(count, end int) []int {
	if end <= 0 {
		return nil
	}
	if count > end {
		count = end
	}
	if count <= 1 {
		if count == 1 {
			return []int{0}
		}
		return nil
	}
	out := make([]int, 0, count)
	seen := make(map[int]bool, count)
	for i := 0; i < count; i++ {
		idx := i * (end - 1) / (count - 1)
		if !seen[idx] {
			seen[idx] = true
			out = append(out, idx)
		}
	}
	return out
}

// dedupSortedInts 升序序列原地去重
func dedupSortedInts(vals []int) []int {
	out := vals[:0]
	for i, v := range vals {
		if i == 0 || v != vals[i-1] {
			out = append(out, v)
		}
	}
	return out
}
