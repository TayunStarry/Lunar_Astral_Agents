package kokoro

import "strings"

// sanitizeText 滤除不可合成字符（Emoji、控制符、零宽/格式符、私用区等），返回净化后的文本
func sanitizeText(text string) string {
	var b strings.Builder
	for _, r := range text {
		if isSynthesizable(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isSynthesizable 判断字符是否适合参与合成：
// 滤除 Emoji、控制符、零宽/格式符、私用区与非法代理区，其余（汉字/字母/数字/标点/空白）保留
func isSynthesizable(r rune) bool {
	switch {
	case r == '\n' || r == '\t' || r == '\r' || r == ' ':
		return true
	case r < 0x20 || (r >= 0x7F && r <= 0x9F): // C0/C1 控制符
		return false
	case r == 0xAD: // 软连字符
		return false
	case r == 0xFEFF: // 零宽不换行空格（BOM）
		return false
	case r >= 0x200B && r <= 0x200F: // 零宽空格 / 零宽连接符 / 零宽不连接符
		return false
	case r == 0x200D: // 零宽连接符（Emoji 组合序列）
		return false
	case r == 0x2060: // 词连接符
		return false
	case r == 0xFE0F: // 变体选择符（Emoji 修饰）
		return false
	case r >= 0x1F000 && r <= 0x1FAFF: // Emoji 主区（含补充符号与扩展象形）
		return false
	case r >= 0x2600 && r <= 0x27BF: // 杂项符号 / 装饰符号（Emoji 常见来源）
		return false
	case r >= 0x2B00 && r <= 0x2BFF: // 杂项符号与箭头（Emoji 常见来源）
		return false
	case r >= 0xE000 && r <= 0xF8FF: // 私用区
		return false
	case r >= 0xF0000 && r <= 0x10FFFF: // 补充私用区
		return false
	case r >= 0xD800 && r <= 0xDFFF: // 非法代理区（防御）
		return false
	default:
		return true
	}
}

// parseProsody 解析文本中的韵律提示符：
//   [↑] 升调：作用于其前紧邻的文本段
//   [↓] 降调：作用于其前紧邻的文本段
//   [•N] 停顿：插入 N*0.01 秒静音（N 为整数）
// 非上述格式的 [内容] 一律按普通文本原样保留。
func parseProsody(text string) []ProsodySeg {
	runes := []rune(text)
	var segs []ProsodySeg
	var cur strings.Builder
	segRise, segFall := false, false
	var pause float32
	// 文本段尚为空时遇到的调性提示，挂起并作用于下一个文本段
	pendingTone := 0 // 0 无，1 升，2 降
	hintSeen := false

	flush := func() {
		// 纯空白且无任何提示的空段不入列
		if strings.TrimSpace(cur.String()) == "" && !segRise && !segFall && pause <= 0 {
			return
		}
		segs = append(segs, ProsodySeg{
			Text:       cur.String(),
			Rise:       segRise,
			Fall:       segFall,
			PauseAfter: pause,
		})
		cur.Reset()
		segRise, segFall = false, false
		pause = 0
	}
	applyPending := func() {
		if pendingTone == 1 {
			segRise, segFall = true, false
		} else if pendingTone == 2 {
			segFall, segRise = true, false
		}
		pendingTone = 0
	}

	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '[' {
			j := i + 1
			for j < len(runes) && runes[j] != ']' {
				j++
			}
			if j < len(runes) {
				inner := string(runes[i+1 : j])
				switch {
				case inner == "↑":
					if cur.Len() > 0 {
						segRise, segFall = true, false // 后出现的调性覆盖前者
					} else {
						pendingTone = 1
					}
					hintSeen = true
					i = j + 1
					continue
				case inner == "↓":
					if cur.Len() > 0 {
						segFall, segRise = true, false
					} else {
						pendingTone = 2
					}
					hintSeen = true
					i = j + 1
					continue
				default:
					if p, ok := parsePauseHint(inner); ok {
						if pendingTone != 0 && cur.Len() == 0 {
							applyPending()
						}
						pause += p
						hintSeen = true
						i = j + 1
						continue
					}
				}
			}
			// 非提示符的 [内容]：作为普通文本写入
		}
		// 遇到普通文本：若此前刚出现提示符，则先收尾上一段
		if hintSeen {
			flush()
			applyPending()
			hintSeen = false
		}
		cur.WriteRune(r)
		i++
	}
	flush()
	return segs
}

// parsePauseHint 解析 [•N] 停顿提示（支持 • 与 · 两种圆点），返回秒数（N*0.01）与是否有效
func parsePauseHint(inner string) (float32, bool) {
	s := inner
	switch {
	case strings.HasPrefix(s, "•"):
		s = strings.TrimPrefix(s, "•")
	case strings.HasPrefix(s, "·"):
		s = strings.TrimPrefix(s, "·")
	default:
		return 0, false
	}
	if s == "" {
		return 0, false
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int64(r-'0')
	}
	if n <= 0 {
		return 0, false
	}
	if n > MaxPauseSteps {
		n = MaxPauseSteps
	}
	return float32(n) * 0.01, true
}

// pauseSamples 将停顿秒数换算为采样数
func pauseSamples(seconds float32) int {
	n := int(float32(SampleRate) * seconds)
	if n < 0 {
		n = 0
	}
	return n
}

// pitchShift 音高偏移（保持时长不变）：factor>1 升调，0<factor<1 降调
// 实现：重采样改变音高 + WSOLA 时间拉伸还原时长
func pitchShift(samples []float32, factor float32) []float32 {
	if factor <= 0 || factor == 1 || len(samples) == 0 {
		return samples
	}
	inPeak := peak(samples)
	// 1. 重采样：factor>1 时以 1/factor 比例缩短，音高升高
	resampled := resampleLinear(samples, 1/factor)
	// 2. 时间拉伸回原时长（保持音高不变）
	stretched := timeStretch(resampled, factor)
	// 3. 峰值对齐，避免音高处理后音量跳变
	if outPeak := peak(stretched); inPeak > 0 && outPeak > 0 {
		scale := inPeak / outPeak
		if scale < 0.5 || scale > 2 {
			scale = 1 // 防御异常比例
		}
		for i := range stretched {
			stretched[i] *= scale
		}
	}
	return stretched
}

// resampleLinear 线性插值重采样，ratio 为输出长度与输入长度之比
func resampleLinear(samples []float32, ratio float32) []float32 {
	n := len(samples)
	if ratio <= 0 || n == 0 {
		return samples
	}
	outLen := int(float32(n) * ratio)
	if outLen < 1 {
		outLen = 1
	}
	out := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		pos := float32(i) / ratio
		idx := int(pos)
		frac := pos - float32(idx)
		if idx >= n-1 {
			out[i] = samples[n-1]
		} else {
			out[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		}
	}
	return out
}

// timeStretch 时域时间拉伸（WSOLA：波形相似重叠相加），rate 为输出长度与输入长度之比，保持音高不变
func timeStretch(samples []float32, rate float32) []float32 {
	n := len(samples)
	if rate <= 0 || n == 0 || rate == 1 {
		return samples
	}
	const (
		winSize  = 960 // 分析窗长 40ms @24kHz
		synthHop = 480 // 合成帧距 20ms
	)
	overlap := winSize - synthHop
	maxSearch := overlap / 2

	if n < winSize {
		// 短片段退化：直接线性拉伸（长度优先）
		return resampleLinear(samples, rate)
	}

	outLen := int(float32(n) * rate)
	if outLen < 1 {
		outLen = 1
	}
	out := make([]float32, outLen+winSize)
	copy(out, samples[:winSize])

	anaHop := int(float32(synthHop)/rate + 0.5) // 分析帧距（四舍五入抑制漂移）
	inPos := anaHop
	outPos := synthHop
	lastEnd := winSize // 已被窗口覆盖的输入末尾
	for inPos+winSize <= n && outPos < outLen {
		// 在搜索范围内寻找与输出重叠区最相似的起点
		best, bestCorr := inPos, float32(-1)
		lo := inPos - maxSearch
		if lo < 0 {
			lo = 0
		}
		hi := inPos + maxSearch
		if hi > n-winSize {
			hi = n - winSize
		}
		for cand := lo; cand <= hi; cand++ {
			var corr float32
			for k := 0; k < overlap; k++ {
				corr += samples[cand+k] * out[outPos+k]
			}
			if corr > bestCorr {
				bestCorr = corr
				best = cand
			}
		}
		// 重叠区交叉淡化，尾部直接拷贝
		for k := 0; k < overlap; k++ {
			w := float32(k+1) / float32(overlap+1)
			out[outPos+k] = out[outPos+k]*(1-w) + samples[best+k]*w
		}
		for k := overlap; k < winSize; k++ {
			out[outPos+k] = samples[best+k]
		}
		lastEnd = best + winSize
		inPos += anaHop
		outPos += synthHop
	}
	// 追加未被窗口覆盖的输入尾部（从输出实际写入末尾接续，避免覆盖上一帧）
	writePos := outPos + overlap
	for lastEnd < n && writePos < outLen+winSize {
		out[writePos] = samples[lastEnd]
		lastEnd++
		writePos++
	}
	// 不足目标长度时以末样本淡出补齐，避免长度漂移与爆音
	if writePos < outLen {
		pad := outLen - writePos
		for i := 0; i < pad; i++ {
			frac := 1 - float32(i+1)/float32(pad+1)
			out[writePos+i] = samples[n-1] * frac
		}
	}
	return out[:outLen]
}

// peak 计算波形峰值
func peak(samples []float32) float32 {
	var p float32
	for _, s := range samples {
		a := s
		if a < 0 {
			a = -a
		}
		if a > p {
			p = a
		}
	}
	return p
}
