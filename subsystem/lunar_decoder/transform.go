package lunardecoder

import "strings"

// opIsBackward 判断操作类型是否为“向后循环位移”。
// 偶数类型（0/2/4/6/8）为向后位移，奇数类型（1/3/5/7/9）为向前位移。
func opIsBackward(op int) bool {
	return op%2 == 0
}

// rotateDir 将字符 c 在 alphabet 中按 count 位循环位移。
// back 为 true 表示向后位移（索引增加），false 表示向前位移（索引减小）；
// reverse 为 true 时方向取反，用于解码过程。
// 若字符 c 不在 alphabet 中则原样返回。
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

// reverseMapApply 将字符 c 映射为反向排序表中的对应字符；不在表中的字符原样返回。
func reverseMapApply(c byte) byte {
	if r, ok := reverseMap[c]; ok {
		return r
	}
	return c
}

// ApplyTransform 依据 8 组操作对字节串 s 执行变换。
//
// decode 为 false 时执行编码（按 0..7 顺序应用各组正向操作）；
// decode 为 true 时执行解码（按 7..0 逆序应用各组逆操作）。
// 该变换是字节串上的双射，因此编码后再解码可完整还原输入。
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

// transformGroup 对字节串应用单个小组的变换。
// reverse 为 true 时执行该小组的逆操作（仅当小组内存在多步操作时次序需要倒置）。
func transformGroup(s []byte, g Group, reverse bool) []byte {
	out := append([]byte(nil), s...)
	back := opIsBackward(g.Op)
	switch g.Op {
	case 0, 1:
		// 数字部分（0-9）循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], digitsAlphabet, g.Count, back, reverse)
		}
	case 2, 3:
		// 字母部分（a-z, A-Z）循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], lettersAlphabet, g.Count, back, reverse)
		}
	case 4, 5:
		// 符号部分（+、/、=）循环位移。
		for i := range out {
			out[i] = rotateDir(out[i], symbolsAlphabet, g.Count, back, reverse)
		}
	case 6, 7:
		// 全字符反向排序表转换 + 循环位移。
		if reverse {
			// 逆操作：先反向位移，再反向排序表（该映射自逆）。
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
		// 全字符集内与下一位索引异或 + 循环位移。
		// 异或作用在 xorAlphabet（64 字符 Base64 字符集）的索引上：两个 6 位索引异或结果
		// 仍在 [0,63]，保证异或产物不越出字符集；不在字符集内的字符（如填充 '='）作为
		// 边界原样保留，链式结构在解码时按逆序恢复。末位不参与异或，保持严格可逆。
		n := len(out)
		if reverse {
			// 逆操作：先反向位移，再逆序索引异或。
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
