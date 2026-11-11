package viewerstore

// text.go 观影正文的通用文本处理：BV 号转调。
//
// 机械内容判定属于观影偏好策略，已上移到 chain/viewer。

import (
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// ==== BV 号规则 ====
// 唯一实现在 internal/textutil/bvid.go：它是无依赖的叶子包，
// 可同时被 engine/resource、本包与 chain/viewer 引用；依赖方向始终是 chain → engine，
// 不会出现 engine 包之间的横向依赖。这里只保留包内转调入口。

// ExtractBVID 从任意文本里提取第一个 BV 号；找不到返回空串。
func ExtractBVID(text string) string { return textutil.ExtractBVID(text) }

// IsBVID 判断整串是否 BV 号（用于摘要库主键判定）。
func IsBVID(key string) bool { return textutil.IsBVID(key) }

// ReplaceBVID 把文本中所有 BV 号交给 repl 转换（批量登记资源句柄用）。
func ReplaceBVID(text string, repl func(string) string) string {
	return textutil.ReplaceBVID(text, repl)
}
