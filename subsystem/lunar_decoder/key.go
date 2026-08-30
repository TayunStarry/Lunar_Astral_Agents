package lunardecoder

import (
	"strconv"
	"strings"
)

// KeyGroups 将 16 位整数密钥处理并拆分为 8 个操作小组。
//
// 密钥处理流程：
//  1. 取 16 位整数的无符号幅度并转换为十进制数字字符串；
//  2. 标准化为 16 位数字：
//     - 不足 16 位：在右侧补充 '0' 直至达到 16 位；
//     - 超过 16 位：从左至右按 16 位长度拆分，最后一段不足 16 位时右侧补 '0'；
//  3. 对每组 16 位密钥执行逐位不进位加法（0-9 范围内），合并为单一 16 位密钥；
//  4. 将合并后的 16 位密钥拆分为 8 个两数字小组，首位为操作类型，次位为操作次数。
//
// 返回 8 个 [Group]；若派生出的操作小组非法则返回 [ErrInvalidKey]。
func KeyGroups(key int16) ([8]Group, error) {
	// 取 16 位无符号幅度，避免负号破坏数字分组。
	u := uint16(key)
	return KeyGroupsFromString(strconv.FormatUint(uint64(u), 10))
}

// KeyGroupsFromString 将十进制数字字符串密钥处理并拆分为 8 个操作小组。
//
// keyStr 长度不限于 16 位：
//   - 不足 16 位：右侧补 '0' 至 16 位；
//   - 超过 16 位：按每段 16 位从左拆分，末段不足 16 位时右侧补 '0'，
//     并对各段逐位不进位加法（0-9 取模）合并为单一 16 位密钥，再拆分为 8 组操作。
//
// 若 keyStr 为空、包含非数字字符或派生小组非法，则返回 [ErrInvalidKey]。
func KeyGroupsFromString(keyStr string) ([8]Group, error) {
	keyStr = strings.TrimSpace(keyStr)
	if keyStr == "" {
		return [8]Group{}, ErrInvalidKey
	}
	for _, r := range keyStr {
		if r < '0' || r > '9' {
			return [8]Group{}, ErrInvalidKey
		}
	}
	return combineKeySegments(keyStr)
}

// combineKeySegments 将十进制数字字符串标准化、合并并拆分为操作小组。
//
// 独立封装便于针对超长字符串（超过 16 位）的拆分与不进位加法路径进行单元测试。
func combineKeySegments(keyStr string) ([8]Group, error) {
	var groups [8]Group

	// 1. 标准化：切分为若干 16 位数字段，段长不足 16 位时右侧补 '0'。
	var segments []string
	rest := keyStr
	for len(rest) > 0 {
		var part string
		if len(rest) > keyGroupSize {
			part = rest[:keyGroupSize]
			rest = rest[keyGroupSize:]
		} else {
			part = rest
			rest = ""
		}
		for len(part) < keyGroupSize {
			part += "0"
		}
		segments = append(segments, part)
	}
	if len(segments) == 0 {
		// 空输入（理论上 key 恒有至少一位）时回退为全零。
		segments = append(segments, strings.Repeat("0", keyGroupSize))
	}

	// 2. 逐位不进位加法（对 10 取模），合并为单一 16 位密钥。
	combined := make([]byte, keyGroupSize)
	for i := range combined {
		combined[i] = '0'
	}
	for _, seg := range segments {
		for i := 0; i < keyGroupSize; i++ {
			sum := int(seg[i]-'0') + int(combined[i]-'0')
			combined[i] = byte(sum%10) + '0'
		}
	}

	// 3. 拆分为 8 个两数字小组。
	for i := 0; i < groupNum; i++ {
		op := int(combined[i*2] - '0')
		count := int(combined[i*2+1] - '0')
		if op < 0 || op > 9 || count < 0 || count > 9 {
			return groups, ErrInvalidKey
		}
		groups[i] = Group{Op: op, Count: count}
	}
	return groups, nil
}
