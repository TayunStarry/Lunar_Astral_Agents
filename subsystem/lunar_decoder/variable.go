package lunardecoder

// 密钥与变换相关常量。
const (
	// keyGroupSize 密钥标准化长度（16 位数字）。
	keyGroupSize = 16
	// groupNum 16 位密钥拆分出的操作小组数量（8 组）。
	groupNum = 8
	// rleMinRun 游程编码的最小游程长度，低于该长度的重复按字面量输出。
	rleMinRun = 4
)

var (
	// digitsAlphabet 数字字符集（操作类型 0/1）。
	digitsAlphabet = "0123456789"

	// lettersAlphabet 字母字符集，小写在前、大写在后（操作类型 2/3）。
	lettersAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// symbolsAlphabet ASCII 符号字符集（操作类型 4/5）。
	symbolsAlphabet = "+/="

	// asciiOrder 按 ASCII 码位升序排列的 Base64 全字符集，
	// 用于反向排序表（操作类型 6/7）与全字符循环位移（操作类型 6-9）。
	// ASCII 升序：'+'(43) '/' '(47) '0'-'9'(48-57) '='(61) 'A'-'Z'(65-90) 'a'-'z'(97-122)。
	asciiOrder = "+/0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// reverseMap 由 asciiOrder 构建的反向排序映射表。
	// 位于位置 i 的字符映射到位置 len(asciiOrder)-1-i 的字符；
	// 该映射是自逆的：应用两次等于恒等变换。
	reverseMap = buildReverseMap(asciiOrder)

	// xorAlphabet 操作 8/9 的异或字符集：64 字符标准 Base64 字符集（不含填充 '='）。
	// 索引空间为 2^6=64，两个索引按位异或的结果仍在 [0,63]，保证异或产物不越出字符集，
	// 因此编码产物始终是 Base64 字符集内的可打印文本，可安全存入 JSON / 文本传输场景。
	xorAlphabet = "+/0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// xorIndex 字符 → xorAlphabet 索引查表；不在字符集内的字符（如填充 '='）为 -1，表示该位不参与异或。
	xorIndex = buildXorIndex(xorAlphabet)
)

// buildReverseMap 依据字符集顺序构建反向排序映射表。
func buildReverseMap(order string) map[byte]byte {
	m := make(map[byte]byte, len(order))
	last := len(order) - 1
	for i := 0; i < len(order); i++ {
		m[order[i]] = order[last-i]
	}
	return m
}

// buildXorIndex 构建字符到 xorAlphabet 索引的查表；非字符集字符映射为 -1（异或边界）。
func buildXorIndex(order string) [256]int {
	var m [256]int
	for i := range m {
		m[i] = -1
	}
	for i := 0; i < len(order); i++ {
		m[order[i]] = i
	}
	return m
}
