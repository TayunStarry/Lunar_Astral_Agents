package lunardecoder

import "strings"

// opIsBackward 偶数操作类型（0/2/4/6/8）为向后位移。
func opIsBackward(op int) bool {
	return op%2 == 0
}

// rotateDir 将字符 c 在 alphabet 中循环位移 count 位；不在 alphabet 中原样返回。
// back 为 true 向后（索引增），reverse 为 true 时方向取反。
func rotateDir(c byte, alphabet string, count int, back, reverse bool) byte {
	idx := strings.IndexByte(alphabet, c)
	if idx < 0 {
		return c
	}
	if reverse {
		back = !back
	}
	n := len(alphabet)
	k := count % n
	var r int
	if back {
		r = (idx + k) % n
	} else {
		r = (idx - k + n) % n
	}
	return alphabet[r]
}

// reverseMapApply 将字符 c 映射为反向排序表中的对应字符。
func reverseMapApply(c byte) byte {
	if r, ok := reverseMap[c]; ok {
		return r
	}
	return c
}

// ApplyTransform 按 8 组操作变换字节串；decode 为 true 时按逆序执行逆操作。
func ApplyTransform(s []byte, groups [8]Group, decode bool) []byte {
	out := append([]byte(nil), s...)
	if decode {
		for i := 7; i >= 0; i-- {
			out = transformGroup(out, groups[i], true)
		}
		return out
	}
	for i := 0; i < 8; i++ {
		out = transformGroup(out, groups[i], false)
	}
	return out
}

// transformGroup 对字节串应用单组操作。
func transformGroup(s []byte, g Group, reverse bool) []byte {
	out := append([]byte(nil), s...)
	back := opIsBackward(g.Op)
	switch g.Op {
	case 0, 1:
		// 数字部分循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], digitsAlphabet, g.Count, back, reverse)
		}
	case 2, 3:
		// 字母部分循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], lettersAlphabet, g.Count, back, reverse)
		}
	case 4, 5:
		// 符号部分循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], symbolsAlphabet, g.Count, back, reverse)
		}
	case 6, 7:
		// 全字符反向排序表转换 + 循环位移。
		if reverse {
			for i := range out {
				out[i] = rotateDir(out[i], asciiOrder, g.Count, back, reverse)
			}
			for i := range out {
				out[i] = reverseMapApply(out[i])
			}
		} else {
			for i := range out {
				out[i] = reverseMapApply(out[i])
			}
			for i := range out {
				out[i] = rotateDir(out[i], asciiOrder, g.Count, back, reverse)
			}
		}
	case 8, 9:
		// 全字符集内与下一位索引异或 + 循环位移（逆序异或用于解码）。
		n := len(out)
		if reverse {
			for i := range out {
				out[i] = rotateDir(out[i], asciiOrder, g.Count, back, reverse)
			}
			for i := n - 2; i >= 0; i-- {
				cur, nxt := xorIndex[out[i]], xorIndex[out[i+1]]
				if cur >= 0 && nxt >= 0 {
					out[i] = xorAlphabet[cur^nxt]
				}
			}
		} else {
			for i := 0; i < n-1; i++ {
				cur, nxt := xorIndex[out[i]], xorIndex[out[i+1]]
				if cur >= 0 && nxt >= 0 {
					out[i] = xorAlphabet[cur^nxt]
				}
			}
			for i := range out {
				out[i] = rotateDir(out[i], asciiOrder, g.Count, back, reverse)
			}
		}
	}
	return out
}
