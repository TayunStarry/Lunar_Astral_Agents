package module

import "strings"

var cnDigits = []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

// numbersToChinese 将文本中的阿拉伯数字转换为中文（cn2an an2cn 子集）
// 支持整数、小数、百分数、分数、序数（第 N）、负数
func numbersToChinese(text string) string {
	var b strings.Builder
	runes := []rune(text)
	i := 0
	n := len(runes)
	for i < n {
		r := runes[i]
		if r == '第' && i+1 < n && isDigit(runes[i+1]) {
			num, next := parseNumber(runes, i+1)
			b.WriteString("第")
			b.WriteString(intToCn(num.intPart))
			i = next
			continue
		}
		if isDigit(r) || (r == '-' && i+1 < n && isDigit(runes[i+1])) {
			neg := false
			if r == '-' {
				neg = true
				i++
			}
			num, next := parseNumber(runes, i)
			// 分数 a/b
			if next < n && runes[next] == '/' && next+1 < n && isDigit(runes[next+1]) {
				den, next2 := parseNumber(runes, next+1)
				if neg {
					b.WriteString("负")
				}
				b.WriteString(intToCn(den.intPart))
				b.WriteString("分之")
				b.WriteString(intToCn(num.intPart))
				i = next2
				continue
			}
			// 百分数
			if next < n && runes[next] == '%' {
				if neg {
					b.WriteString("负")
				}
				b.WriteString("百分之")
				b.WriteString(numberToCn(num))
				i = next + 1
				continue
			}
			if neg {
				b.WriteString("负")
			}
			b.WriteString(numberToCn(num))
			i = next
			continue
		}
		b.WriteRune(r)
		i++
	}
	return b.String()
}

// numVal 解析的数值
type numVal struct {
	intPart int64
	frac    string
}

// parseNumber 从 runes[pos] 开始解析数字，返回数值与下一个位置
func parseNumber(runes []rune, pos int) (numVal, int) {
	var intPart int64
	for pos < len(runes) && isDigit(runes[pos]) {
		intPart = intPart*10 + int64(runes[pos]-'0')
		pos++
	}
	var frac string
	if pos < len(runes) && runes[pos] == '.' {
		pos++
		fracStart := pos
		for pos < len(runes) && isDigit(runes[pos]) {
			pos++
		}
		frac = string(runes[fracStart:pos])
	}
	return numVal{intPart: intPart, frac: frac}, pos
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// numberToCn 数值转中文（含小数）
func numberToCn(v numVal) string {
	s := intToCn(v.intPart)
	if s == "" {
		s = "零"
	}
	if v.frac != "" {
		s += "点"
		for _, r := range v.frac {
			s += cnDigits[r-'0']
		}
	}
	return s
}

// intToCn 整数转中文（标准位权法，支持到万亿）
func intToCn(num int64) string {
	if num == 0 {
		return "零"
	}
	// 按 4 位分组，groups[3] 为最低组（个位），groups[0] 为最高组
	var groups [4]int64
	gi := 3
	for num > 0 && gi >= 0 {
		groups[gi] = num % 10000
		num /= 10000
		gi--
	}
	bigUnits := []string{"", "万", "亿"}
	var parts []string
	started := false
	for i := 0; i < 4; i++ {
		g := groups[i]
		if g == 0 {
			continue
		}
		bi := 3 - i
		if started && g < 1000 {
			parts = append(parts, "零")
		}
		parts = append(parts, groupToCn(g))
		if bi > 0 {
			parts = append(parts, bigUnits[bi])
		}
		started = true
	}
	res := strings.Join(parts, "")
	// 十位开头的特例（如 10 -> 十，而非 一十）
	if strings.HasPrefix(res, "一十") && !strings.HasPrefix(res, "一百") && !strings.HasPrefix(res, "一千") &&
		!strings.HasPrefix(res, "一万") && !strings.HasPrefix(res, "一亿") {
		res = res[1:]
	}
	return res
}

// groupToCn 转换 0~9999 为一组中文
func groupToCn(g int64) string {
	if g == 0 {
		return ""
	}
	digits := []int{int(g / 1000 % 10), int(g / 100 % 10), int(g / 10 % 10), int(g % 10)}
	units := []string{"千", "百", "十", ""}
	var parts []string
	started := false
	needZero := false
	for i, d := range digits {
		if d > 0 {
			if needZero && started {
				parts = append(parts, "零")
			}
			needZero = false
			started = true
			// 10~19 读作 十X（十位为 1 且前位为空）
			if d == 1 && i == 2 && g < 100 {
				parts = append(parts, "十")
			} else {
				parts = append(parts, cnDigits[d])
				if units[i] != "" {
					parts = append(parts, units[i])
				}
			}
		} else if started {
			needZero = true
		}
	}
	return strings.Join(parts, "")
}
