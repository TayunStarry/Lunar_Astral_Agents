package lunardecoder

import (
	"strconv"
	"strings"
)

// KeyGroups 返回密钥首轮派生的 8 组操作。
func KeyGroups(key int16) ([8]Group, error) {
	rounds, err := KeyRounds(key)
	if err != nil {
		return [8]Group{}, err
	}
	return rounds[0].Groups, nil
}

// KeyGroupsFromString 返回密钥字符串首轮派生的 8 组操作。
func KeyGroupsFromString(keyStr string) ([8]Group, error) {
	rounds, err := KeyRoundsFromString(keyStr)
	if err != nil {
		return [8]Group{}, err
	}
	return rounds[0].Groups, nil
}

// KeyRounds 将 16 位整数密钥（取 16 位无符号幅度）处理为若干 16 字符轮。
func KeyRounds(key int16) ([]Round, error) {
	return KeyRoundsFromString(strconv.FormatUint(uint64(uint16(key)), 10))
}

// KeyRoundsFromString 将密钥字符串处理为若干 16 字符轮。
//
// 密钥字符集为 65 字符（asciiOrder）。规则：
//  1. 按每 16 字符拆分一轮，多轮按序对前一轮结果重复加密；
//  2. 不足 16 字符的轮用圆周率小数补齐，d 个字符从第 (d-1)*20+1 位起取；
//  3. 每轮 16 字符拆 8 组（每组操作类型位 + 执行次数位）：
//     控制位取值 v（数字取数字值、符号取字符集编码号）得 Op=v%10，v/10 叠加到次数位；
//     执行次数位直接取字符集编码号，并与控制位的整十倍数相加。
//
// 密钥为空或含字符集外字符时返回 [ErrInvalidKey]。
func KeyRoundsFromString(keyStr string) ([]Round, error) {
	return keyRounds(keyStr)
}

// keySpec 内部入口，返回全部加密轮。
func keySpec(keyStr string) ([]Round, error) {
	return keyRounds(keyStr)
}

// keyRounds 校验字符集并按 16 字符拆分轮次。
func keyRounds(keyStr string) ([]Round, error) {
	keyStr = strings.TrimSpace(keyStr)
	if keyStr == "" {
		return nil, ErrInvalidKey
	}
	for i := 0; i < len(keyStr); i++ {
		if keyCharIndex[keyStr[i]] < 0 {
			return nil, ErrInvalidKey
		}
	}

	var rounds []Round
	for rest := keyStr; len(rest) > 0; {
		var part string
		if len(rest) > keyGroupSize {
			part = rest[:keyGroupSize]
			rest = rest[keyGroupSize:]
		} else {
			part = rest
			rest = ""
		}
		round, err := makeRound(part)
		if err != nil {
			return nil, err
		}
		rounds = append(rounds, round)
	}
	return rounds, nil
}

// makeRound 将一段字符构造为完整 16 字符轮，不足位用圆周率补齐。
func makeRound(part string) (Round, error) {
	d := len(part)
	padded := part
	start := (d - 1) * piOffsetStep
	for len(padded) < keyGroupSize {
		idx := start + (len(padded) - d)
		if idx >= len(piDecimals) {
			return Round{}, ErrInvalidKey
		}
		padded += string(piDecimals[idx])
	}

	groups, err := groupsFromKey(padded)
	if err != nil {
		return Round{}, err
	}
	return Round{Key: padded, Groups: groups}, nil
}

// groupsFromKey 将 16 字符密钥拆分为 8 组两字符操作。
func groupsFromKey(key string) ([8]Group, error) {
	var groups [8]Group
	for i := 0; i < groupNum; i++ {
		g, err := groupFromChars(key[i*2], key[i*2+1])
		if err != nil {
			return [8]Group{}, err
		}
		groups[i] = g
	}
	return groups, nil
}

// groupFromChars 由一组两字符派生操作小组：
// 控制位取值 v -> Op=v%10，v/10 叠加到执行次数；次数位取字符集编码号。
func groupFromChars(opChar, cntChar byte) (Group, error) {
	opv := charValue(opChar)
	cntv := charCode(cntChar)
	if opv < 0 || cntv < 0 {
		return Group{}, ErrInvalidKey
	}
	op := opv % 10
	extra := opv / 10
	return Group{Op: op, Count: cntv + extra}, nil
}
