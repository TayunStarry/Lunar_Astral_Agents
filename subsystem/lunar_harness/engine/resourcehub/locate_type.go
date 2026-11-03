package resourcehub

// locate_type.go 文件定位器的结果与错误类型。

import (
	"fmt"
	"strings"
)

// Match 一次文件定位结果
type Match struct {
	// Abs 绝对路径
	Abs string
	// Rel 相对 LocalDir 的路径（/ 分隔）
	Rel string
	// Handle 已登记的资源句柄（未登记时为空）
	Handle string
}

// AmbiguousError 同名歧义：附候选相对路径供调用方选择
type AmbiguousError struct {
	// Input 原始输入
	Input string
	// Candidates 候选文件（相对 LocalDir，已排序去重）
	Candidates []string
}

// Error 生成带候选列表的歧义提示
func (e *AmbiguousError) Error() string {
	lines := make([]string, 0, len(e.Candidates))
	for index, candidate := range e.Candidates {
		if index >= maxCandidateDisplay {
			break
		}
		lines = append(lines, "- "+candidate)
	}
	extra := ""
	if len(e.Candidates) > maxCandidateDisplay {
		extra = fmt.Sprintf("\n…另有 %d 个候选未列出", len(e.Candidates)-maxCandidateDisplay)
	}
	return fmt.Sprintf("「%s」匹配到多个文件，请改用完整路径：\n%s%s", e.Input, strings.Join(lines, "\n"), extra)
}
