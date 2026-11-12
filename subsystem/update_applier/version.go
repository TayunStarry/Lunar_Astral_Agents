package main

import (
	"strconv"
	"strings"
	"time"
)

// ==== 版本比较 ====

// tagRelation 版本关系常量。
const (
	relationNewer   = "newer"
	relationSame    = "same"
	relationOlder   = "older"
	relationUnknown = "unknown"
)

// defaultTag 生成默认版本号（vYYYY.MM.DD-HHMM）。
func defaultTag() string {
	return time.Now().Format("v2006.01.02-1504")
}

// CompareTagNatural 按数字段自然序比较两个 tag，返回 -1 / 0 / 1。
// 支持 v2026.11.11-1530 与 v1.2.3 两类格式；数字段按整数比较。
func CompareTagNatural(a, b string) int {
	pa := tagTokens(a)
	pb := tagTokens(b)
	n := len(pa)
	if len(pb) < n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		na, aNum := strconv.Atoi(pa[i])
		nb, bNum := strconv.Atoi(pb[i])
		if aNum == nil && bNum == nil {
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			continue
		}
		// 数字段优先按数值比较；非数字段按字典序。
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	if len(pa) == len(pb) {
		return 0
	}
	if len(pa) < len(pb) {
		return -1
	}
	return 1
}

// tagTokens 把 tag 拆成数字段与非数字段，忽略前导 v 与分隔符差异。
func tagTokens(tag string) []string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	var out []string
	cur := strings.Builder{}
	curDigit := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range tag {
		switch {
		case r >= '0' && r <= '9':
			if cur.Len() > 0 && !curDigit {
				flush()
			}
			curDigit = true
			cur.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == '+' || r == ' ':
			flush()
			curDigit = false
		default:
			if cur.Len() > 0 && curDigit {
				flush()
			}
			curDigit = false
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// VersionRelation 返回目标相对当前版本的关系。
func VersionRelation(current, target string) string {
	if current == "" || target == "" {
		return relationUnknown
	}
	switch CompareTagNatural(current, target) {
	case 0:
		return relationSame
	case -1:
		return relationNewer
	default:
		return relationOlder
	}
}

// relationLabel 返回关系的中文标签。
func relationLabel(relation string) string {
	switch relation {
	case relationNewer:
		return "可升级"
	case relationSame:
		return "重装"
	case relationOlder:
		return "降级"
	default:
		return "未知"
	}
}
