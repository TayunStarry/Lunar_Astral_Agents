package handlers

import "regexp"

// compileRegex 编译正则表达式（内部辅助函数）
func compileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}
