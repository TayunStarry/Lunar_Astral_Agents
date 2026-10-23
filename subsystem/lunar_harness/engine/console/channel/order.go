package channel

// ==== 发布排序（通道无关的公共契约） ====
//
// Gitee 的发布列表接口默认 direction=asc：**响应顺序是最旧在前**，而不是最新在前。
// 更新包与 LTPX 两条通道都曾假设「响应即倒序」，于是「取最新」取到的是最旧的一条
// ——实测：仓库里同时存在 v2026.10.15-2232 与 v2026.10.15-2250，拉取却拿到 2232。
//
// 因此凡是「取最新」的地方都必须自己排序：先看创建时间，时间缺失或相同时退化为
// 标签自然序比较（数字段按数值比，避免 "-930" 被当成大于 "-1030"）。

import (
	"strings"
	"time"
)

// ParseReleaseTime 解析 Gitee 发布的时间戳（RFC3339，如 2026-10-15T22:50:12+08:00）。
// 为空或格式不符时返回 false，调用方改用标签比较兜底。
func ParseReleaseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// CompareTagNatural 标签自然序比较：逐个字符扫描，连续数字段按数值比较。
// 返回 -1（a 更旧/更小）、0（相等）、1（a 更新/更大）。
//
// 为什么不用字典序：标签形如 v2026.10.15-930 与 v2026.10.15-1030，
// 字典序会判 "930" > "1030"（首字符 '9' > '1'），把时间更晚的版本判成更旧。
func CompareTagNatural(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			// 取出两侧完整数字段（跳过前导零后比较长度，再逐位比较）
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if cmp := compareNumeric(a[si:i], b[sj:j]); cmp != 0 {
				return cmp
			}
			continue
		}
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	}
	return 0
}

// Newer 判断发布 a 是否比 b 新：优先创建时间，时间缺失或相同时按标签自然序。
// createdA/createdB 为 Gitee 返回的 created_at 原值，tagA/tagB 为 tag 名。
func Newer(createdA, tagA, createdB, tagB string) bool {
	ta, oka := ParseReleaseTime(createdA)
	tb, okb := ParseReleaseTime(createdB)
	if oka && okb && !ta.Equal(tb) {
		return ta.After(tb)
	}
	return CompareTagNatural(tagA, tagB) > 0
}

// isDigit ASCII 数字判定（标签仅含 ASCII，无需 unicode 处理）
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// compareNumeric 比较两个纯数字字符串（忽略前导零）
func compareNumeric(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}
