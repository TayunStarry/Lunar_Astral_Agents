package creation

// journal_session.go 运行期的参考资料 / 错误 / 备忘收集（内存态，轮次结束时归档）。

import "strings"

// addReference 记录一条参考资料（自动去重与限长）。
func (s *session) addReference(reference string) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return
	}
	for _, seen := range s.refs {
		if seen == reference {
			return
		}
	}
	s.refs = append(s.refs, clipAtSentence(reference, maxReferenceRunes))
	if len(s.refs) > maxReferences {
		s.refs = s.refs[len(s.refs)-maxReferences:]
	}
}

// addError 记录一条错误（自动限长）。
func (s *session) addError(err error) {
	if err == nil {
		return
	}
	s.errs = append(s.errs, clipAtSentence(err.Error(), maxErrorRunes))
	if len(s.errs) > maxErrors {
		s.errs = s.errs[len(s.errs)-maxErrors:]
	}
}

// addMemo 记录一条遗留备忘。
func (s *session) addMemo(memo string) {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return
	}
	s.memos = append(s.memos, clipAtSentence(memo, maxMemoRunes))
}

// mergeDirectives 把本轮创作参考并入日志：去重、限长，只保留最近的若干条。
func mergeDirectives(existing, incoming []string) []string {
	out := make([]string, 0, maxDirectives)
	seen := map[string]bool{}
	for _, item := range append(append([]string{}, existing...), incoming...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		item = clipAtSentence(item, maxDirectiveRunes)
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(out) > maxDirectives {
		out = out[len(out)-maxDirectives:]
	}
	return out
}
