package fileflow

// review.go 操作后审查：把「规划阶段声明的预期」与「操作后的真实文件状态」做确定性比对。
// 不使用模型，不依赖调用方自觉；预期未声明时由调用方传入默认预期。

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FilterExpects 只保留目标路径与本次操作相关的预期（affected 为相对本地目录的路径集合）。
func FilterExpects(expects []Expect, affected map[string]bool) []Expect {
	if len(expects) == 0 {
		return nil
	}
	out := make([]Expect, 0, len(expects))
	for _, expect := range expects {
		path := strings.TrimSpace(expect.Path)
		if path == "" {
			continue
		}
		_, rel, err := Resolve(path)
		if err != nil {
			// 解析失败的预期交给 Evaluate 报出明确失败原因
			out = append(out, expect)
			continue
		}
		if len(affected) > 0 && !affected[rel] {
			continue
		}
		out = append(out, expect)
	}
	return out
}

// MergeExpects 合并显式预期与默认预期：同一路径上显式预期优先，默认预期只补充缺口。
func MergeExpects(explicit, defaults []Expect) []Expect {
	out := make([]Expect, 0, len(explicit)+len(defaults))
	keyOf := func(expect Expect) string {
		_, rel, err := Resolve(expect.Path)
		if err != nil {
			return "path:" + strings.TrimSpace(expect.Path)
		}
		return rel
	}
	seen := map[string]bool{}
	for _, expect := range explicit {
		seen[keyOf(expect)] = true
		out = append(out, expect)
	}
	for _, expect := range defaults {
		if seen[keyOf(expect)] {
			continue
		}
		out = append(out, expect)
	}
	return out
}

// EvaluateAndRollback 校验预期；不一致时立即按快照回滚。
func EvaluateAndRollback(snapshot Snapshot, explicit, defaults []Expect, affected map[string]bool) Review {
	expects := MergeExpects(FilterExpects(explicit, affected), FilterExpects(defaults, affected))
	review := Evaluate(expects)
	review.Snapshot = snapshot.Dir
	if review.OK {
		return review
	}
	if len(snapshot.Entries) == 0 {
		review.RollbackError = "没有可还原的快照条目"
	} else if restoreErr := Restore(snapshot); restoreErr != nil {
		review.RollbackError = restoreErr.Error()
	} else {
		review.RolledBack = true
	}
	review.Context = buildContext(snapshot, review)
	return review
}

// Evaluate 逐条校验预期。
func Evaluate(expects []Expect) Review {
	review := Review{OK: true}
	for _, expect := range expects {
		review.Checked++
		failure, ok := checkExpect(expect)
		if !ok {
			review.OK = false
			review.Failures = append(review.Failures, failure)
		}
	}
	return review
}

// checkExpect 校验单条预期；返回失败项与是否通过。
func checkExpect(expect Expect) (Failure, bool) {
	path := strings.TrimSpace(expect.Path)
	if path == "" {
		return Failure{Reason: "预期缺少 path"}, false
	}
	abs, rel, err := Resolve(path)
	if err != nil {
		return Failure{Path: path, Reason: "预期路径无效: " + err.Error()}, false
	}
	info, statErr := os.Stat(abs)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return Failure{Path: rel, Reason: "读取目标状态失败: " + statErr.Error()}, false
	}
	if expect.Exists != nil {
		if *expect.Exists && !exists {
			return Failure{Path: rel, Reason: "预期文件应存在，实际不存在"}, false
		}
		if !*expect.Exists && exists {
			return Failure{Path: rel, Reason: "预期文件应不存在，实际仍存在"}, false
		}
	}
	if !exists {
		if len(expect.Contains) > 0 || len(expect.NotContains) > 0 ||
			expect.MinBytes != nil || expect.MaxBytes != nil {
			return Failure{Path: rel, Reason: "文件不存在，无法校验内容"}, false
		}
		return Failure{}, true
	}
	if info.IsDir() {
		// 目录只支持存在性校验；要求内容或字节数时显式失败，避免误判为通过
		if len(expect.Contains) > 0 || len(expect.NotContains) > 0 ||
			expect.MinBytes != nil || expect.MaxBytes != nil {
			return Failure{Path: rel, Reason: "目标是目录，无法校验文件内容"}, false
		}
		return Failure{}, true
	}
	if expect.MinBytes != nil && info.Size() < *expect.MinBytes {
		return Failure{Path: rel, Reason: fmt.Sprintf("字节数 %d 小于预期下限 %d", info.Size(), *expect.MinBytes)}, false
	}
	if expect.MaxBytes != nil && info.Size() > *expect.MaxBytes {
		return Failure{Path: rel, Reason: fmt.Sprintf("字节数 %d 超过预期上限 %d", info.Size(), *expect.MaxBytes)}, false
	}
	if len(expect.Contains) == 0 && len(expect.NotContains) == 0 {
		return Failure{}, true
	}
	for _, needle := range expect.Contains {
		found, scanErr := fileContains(abs, needle)
		if scanErr != nil {
			return Failure{Path: rel, Reason: "读取内容失败: " + scanErr.Error()}, false
		}
		if !found {
			return Failure{Path: rel, Reason: "缺少预期内容「" + preview(needle) + "」"}, false
		}
	}
	for _, needle := range expect.NotContains {
		found, scanErr := fileContains(abs, needle)
		if scanErr != nil {
			return Failure{Path: rel, Reason: "读取内容失败: " + scanErr.Error()}, false
		}
		if found {
			return Failure{Path: rel, Reason: "出现禁止内容「" + preview(needle) + "」"}, false
		}
	}
	return Failure{}, true
}

// fileContains 流式扫描文件是否包含指定片段（不受文件大小限制）。
func fileContains(abs, needle string) (bool, error) {
	if needle == "" {
		return true, nil
	}
	handle, err := os.Open(abs)
	if err != nil {
		return false, err
	}
	defer handle.Close()
	const chunkSize = 1 << 20
	buf := make([]byte, chunkSize)
	carry := make([]byte, 0, len(needle))
	for {
		read, readErr := handle.Read(buf)
		if read > 0 {
			window := append(append([]byte(nil), carry...), buf[:read]...)
			if bytes.Contains(window, []byte(needle)) {
				return true, nil
			}
			keep := len(needle) - 1
			if keep > len(window) {
				keep = len(window)
			}
			carry = append(carry[:0], window[len(window)-keep:]...)
		}
		if readErr == io.EOF {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
	}
}

// buildContext 生成交给下一轮流程节点的审查上下文。
func buildContext(snapshot Snapshot, review Review) string {
	var builder strings.Builder
	builder.WriteString("文件操作审查未通过（操作: ")
	builder.WriteString(snapshot.Op)
	builder.WriteString("，目标: ")
	builder.WriteString(snapshot.Target)
	builder.WriteString("）。")
	if review.RolledBack {
		builder.WriteString("已回滚到操作前版本：")
	} else if review.RollbackError != "" {
		builder.WriteString("回滚失败，文件可能停留在错误状态：")
	}
	for index, failure := range review.Failures {
		if index > 0 {
			builder.WriteString("；")
		}
		builder.WriteString(failure.Path)
		builder.WriteString(" ")
		builder.WriteString(failure.Reason)
	}
	if review.RollbackError != "" {
		builder.WriteString("；回滚错误：")
		builder.WriteString(review.RollbackError)
	}
	if review.Snapshot != "" {
		builder.WriteString("；快照：")
		builder.WriteString(filepath.ToSlash(review.Snapshot))
	}
	return builder.String()
}

// preview 截断预期片段，避免错误信息过长。
func preview(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= 80 {
		return string(runes)
	}
	return string(runes[:80]) + "…"
}
