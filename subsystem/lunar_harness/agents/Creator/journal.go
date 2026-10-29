package Creator

// journal.go 创作规划日志：读取、保存与格式化。
// 日志完整记录上一轮的规划、遗留备忘、参考资料与错误，供调度者下一轮统一推理。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// loadJournal 读取规划日志；不存在或损坏时返回空日志。
func loadJournal() journal {
	raw, ok := persistence.ReadFileBytes(journalFile)
	if !ok {
		return journal{Version: 1}
	}
	var saved journal
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[创作] 规划日志损坏，已按空日志继续: %v", err)
		return journal{Version: 1}
	}
	saved.Version = 1
	return saved
}

// saveJournal 原子写入规划日志。
func saveJournal(j journal) {
	j.Version = 1
	j.UpdatedAt = nowUnix()
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		logger.Warn(Module, "[创作] 规划日志序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(journalFile, raw); err != nil {
		logger.Warn(Module, "[创作] 规划日志写入失败: %v", err)
	}
}

// formatJournal 把日志整理成调度者可读的历史信息。
func formatJournal(j journal) string {
	var b strings.Builder
	b.WriteString("【上一轮规划】\n")
	if len(j.LastPlans) == 0 {
		b.WriteString("（空）\n")
	} else {
		for i, step := range j.LastPlans {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, agentLabels[step.Agent], step.Task)
		}
	}
	b.WriteString("\n【上一轮遗留的备忘与期望】\n")
	writeList(&b, j.Memos)
	b.WriteString("\n【上一轮查询到的参考资料】\n")
	writeList(&b, j.References)
	b.WriteString("\n【上一轮的错误与异常】\n")
	writeList(&b, j.Errors)
	if strings.TrimSpace(j.LastSummary) != "" {
		b.WriteString("\n【上一轮收尾】\n")
		b.WriteString(previewRunes(j.LastSummary, 600))
		b.WriteByte('\n')
	}
	return b.String()
}

func writeList(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("（无）\n")
		return
	}
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(previewRunes(item, maxReferenceRunes))
		b.WriteByte('\n')
	}
}

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
	s.refs = append(s.refs, previewRunes(reference, maxReferenceRunes))
	if len(s.refs) > maxReferences {
		s.refs = s.refs[len(s.refs)-maxReferences:]
	}
}

// addError 记录一条错误（自动限长）。
func (s *session) addError(err error) {
	if err == nil {
		return
	}
	s.errs = append(s.errs, previewRunes(err.Error(), 300))
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
	s.memos = append(s.memos, previewRunes(memo, 300))
}
