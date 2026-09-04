package lunardecoder

// 密钥与扩散相关常量。
const (
	// keyGroupSize 每轮密钥长度（16 字符）。
	keyGroupSize = 16
	// groupNum 每轮拆分出的操作小组数量（8 组）。
	groupNum = 8
	// rleMinRun 游程编码的最小游程长度。
	rleMinRun = 4
	// piOffsetStep 圆周率补齐起始位步进，起始位 = (字符数-1)*piOffsetStep + 1。
	piOffsetStep = 20
	// diffuseBlock 扩散层 Feistel 块大小（字节）。
	diffuseBlock = 16
	// diffuseRounds 扩散层 Feistel 子轮数。
	diffuseRounds = 8
)

var (
	// digitsAlphabet 数字字符集（操作 0/1）。
	digitsAlphabet = "0123456789"
	// lettersAlphabet 字母字符集，小写在前、大写在后（操作 2/3）。
	lettersAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// symbolsAlphabet 符号字符集（操作 4/5）。
	symbolsAlphabet = "+/="
	// asciiOrder 按 ASCII 升序排列的 65 字符集，用于反向映射、循环位移与密钥字符集。
	asciiOrder = "+/0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// reverseMap asciiOrder 的反向排序映射表（自逆）。
	reverseMap = buildReverseMap(asciiOrder)
	// xorAlphabet 操作 8/9 的 64 字符异或集（标准 Base64 不含 '='）。
	xorAlphabet = "+/0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// xorIndex 字符到 xorAlphabet 索引查表，无效字符为 -1。
	xorIndex = buildXorIndex(xorAlphabet)
	// piDecimals 圆周率小数部分，用于密钥不足位填补。
	piDecimals = "14159265358979323846264338327950288419716939937510582097494459230781640628620899862803482534211706798214808651328230664709384460955058223172535940812848111745028410270193852110555964462294895493038196442881097566593344612847564823378678316527120190914564856692346034861045432664821339360726024914127372458700660631558817"
	// keyCharIndex 字符到密钥字符集编码号查表，无效字符为 -1。
	keyCharIndex = buildKeyCharIndex(asciiOrder)
	// keyIndexBase 密钥字符集数量（65）。
	keyIndexBase = len(asciiOrder)
)

// buildKeyCharIndex 构建字符到字符集索引的查表，非字符集字符映射为 -1。
func buildKeyCharIndex(order string) [256]int {
	var m [256]int
	for i := range m {
		m[i] = -1
	}
	for i := 0; i < len(order); i++ {
		m[order[i]] = i
	}
	return m
}

// charValue 控制位取值：数字取数字值，非数字取字符集编码号，无效字符返回 -1。
func charValue(ch byte) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	return keyCharIndex[ch]
}

// charCode 次数位取值：直接取字符集编码号，无效字符返回 -1。
func charCode(ch byte) int {
	return keyCharIndex[ch]
}

// buildReverseMap 依据字符集构建反向排序映射表。
func buildReverseMap(order string) map[byte]byte {
	m := make(map[byte]byte, len(order))
	last := len(order) - 1
	for i := 0; i < len(order); i++ {
		m[order[i]] = order[last-i]
	}
	return m
}

// buildXorIndex 构建字符到 xorAlphabet 索引的查表，非字符集字符映射为 -1。
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
