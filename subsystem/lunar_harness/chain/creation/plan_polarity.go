package creation

// plan_polarity.go 心情极性判定：决定本轮续做未完成事项还是换新。
//
// 判定算法（可测、无散落魔法数字）：
//  1. 取正向情绪集合里权重最大的分量 p，负向集合里权重最大的分量 n；
//  2. 极性差值 d = p - n；
//  3. |d| 达到阈值才算倾向明确：d >= 阈值 → 偏正向，d <= -阈值 → 偏负面；
//     否则视为中性，按偏正向处理（保守续做，避免未完成事项被无谓丢弃）。
//
// 阈值来源：心情是 11 类 softmax 分布（见 chain/wishchain 的基础情绪词表），
// 均匀基线为单类份额 1/N。只有当一极的最强分量比另一极高出至少一个均匀份额，
// 才算脱离噪声区间；这样主导标签在相邻两轮之间轻微抖动时不会反复切换创作事项。

import "LunarSubsystem/LunarHarness/chain/wishchain"

// polarityOf 按最强的正 / 负向情绪分量判定极性。
func polarityOf(snapshot wishchain.MoodSnapshot) MoodPolarity {
	if len(snapshot.Weights) == 0 {
		return polarityByLabel(snapshot.Label)
	}
	positive := strongestWeight(snapshot.Weights, positiveMoodLabels)
	negative := strongestWeight(snapshot.Weights, negativeMoodLabels)
	threshold := 1 / float64(len(snapshot.Weights))
	diff := positive - negative
	switch {
	case diff >= threshold:
		return MoodPositive
	case diff <= -threshold:
		return MoodNegative
	default:
		return MoodNeutral
	}
}

// strongestWeight 返回指定标签集合中的最大权重；集合缺失时返回 0。
func strongestWeight(weights map[string]float64, labels []string) float64 {
	strongest := 0.0
	for _, label := range labels {
		if weight := weights[label]; weight > strongest {
			strongest = weight
		}
	}
	return strongest
}

// polarityByLabel 权重缺失时按主导标签所属极性兜底；未知标签按偏正向处理。
func polarityByLabel(label string) MoodPolarity {
	for _, item := range negativeMoodLabels {
		if item == label {
			return MoodNegative
		}
	}
	return MoodPositive
}

// polarityReason 生成可读的选定依据，说明本轮为什么续做或换新。
func polarityReason(polarity MoodPolarity, resumed bool) string {
	switch {
	case resumed && polarity == MoodPositive:
		return "心情偏正向，继续此前未完成的创作事项"
	case resumed:
		return "心情没有明显负向，继续此前未完成的创作事项"
	case polarity == MoodNegative:
		return "心情偏负面，暂存未完成事项并换一个新事项"
	case polarity == MoodPositive:
		return "心情偏正向但没有未完成事项，随机开启一个新事项"
	default:
		return "心情没有明显负向，随机开启一个新事项"
	}
}
