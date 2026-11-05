package textutil

// bvid.go B 站视频号（BV 号）的解析与替换。
//
// 全仓 BV 号规则唯一实现在此：engine/resource、engine/viewerstore 与 chain/viewer
// 均引用本文件，禁止各自再建一份正则。放在 internal 纯文本层而非 engine 内某个包，
// 是因为它必须是无依赖的叶子包：依赖方向只允许 chain → engine 单向，
// 规则落在 engine 任一侧都会给上层带来不必要的横向依赖。

import (
	"regexp"
	"strings"
)

var (
	// bvidRe 从任意文本里提取 BV 号
	bvidRe = regexp.MustCompile(`(BV[0-9A-Za-z]{10})`)
	// fullBVIDRe 匹配「整串就是一个 BV 号」的形态
	fullBVIDRe = regexp.MustCompile(`^BV[0-9A-Za-z]+$`)
)

// ExtractBVID 从任意文本里提取第一个 BV 号；找不到返回空串。
func ExtractBVID(text string) string { return bvidRe.FindString(text) }

// IsBVID 判断整串是否 BV 号（用于摘要库主键判定）。
func IsBVID(key string) bool { return fullBVIDRe.MatchString(strings.TrimSpace(key)) }

// ReplaceBVID 把文本中所有 BV 号交给 repl 转换（批量登记资源句柄用）。
func ReplaceBVID(text string, repl func(string) string) string {
	return bvidRe.ReplaceAllStringFunc(text, repl)
}
