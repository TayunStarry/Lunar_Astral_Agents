package Creator

// journal.go 流程日志：读取、保存、按模式留存与格式化。
//
// 两种留存规则：
//   - Creator 模式：完整规划、备忘、参考资料与错误全程保存，提示词侧只取最近若干条内容级操作记录；
//   - 指派模式：规划类内容在保存前清空，操作记录来自增量数据库，按需取最近若干条。
//
// 操作记录已改为内容级增量（flow_ops / flow_deltas 表），日志不再保存自然语言操作摘要。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// journalPath 返回指定模式的日志路径。
func journalPath(mode fileflow.Mode) string {
	if mode == fileflow.ModeAssigned {
		return assignedJournalFile
	}
	return journalFile
}

// loadJournal 读取规划日志；不存在或损坏时返回空日志。
func loadJournal(path string) journal {
	raw, ok := persistence.ReadFileBytes(path)
	if !ok {
		return journal{Version: 1}
	}
	var saved journal
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[自由创作] 规划日志损坏，已按空日志继续: %v", err)
		return journal{Version: 1}
	}
	saved.Version = 1
	return saved
}

// saveJournal 原子写入规划日志（调用方已按模式裁剪）。
func saveJournal(path string, j journal) {
	j.Version = 1
	j.UpdatedAt = nowUnix()
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		logger.Warn(Module, "[自由创作] 规划日志序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(path, raw); err != nil {
		logger.Warn(Module, "[自由创作] 规划日志写入失败: %v", err)
	}
}

// pruneJournal 按模式裁剪日志：
//   - Creator 模式保留全部字段；
//   - 指派模式清空规划类内容（操作记录由增量库按需查询，不写入日志）。
func pruneJournal(j journal, mode fileflow.Mode) journal {
	j.Mode = string(mode)
	j.Version = 1
	if mode == fileflow.ModeAssigned {
		return journal{
			Version:   1,
			Mode:      string(mode),
			UpdatedAt: j.UpdatedAt,
		}
	}
	return j
}

// formatJournal 把日志整理成调度者可读的历史信息（按模式去噪）。
// recent 为增量库中的最近操作记录（已按条数裁剪），两种模式都只注入这些内容级差异。
func formatJournal(j journal, recent []fileflow.OpRecord) string {
	if j.Mode == string(fileflow.ModeAssigned) {
		var b strings.Builder
		b.WriteString("【指派模式 · 本流程与历史操作的内容差异】\n")
		b.WriteString(formatRecentOps(recent))
		return b.String()
	}
	var b strings.Builder
	b.WriteString("【上一轮规划】\n")
	if len(j.LastPlans) == 0 {
		b.WriteString("（空）\n")
	} else {
		for i, step := range j.LastPlans {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, agentLabels[step.Agent], step.Task)
			if len(step.Expect) > 0 {
				fmt.Fprintf(&b, "   预期：%s\n", formatExpects(step.Expect))
			}
		}
	}
	b.WriteString("\n【上一轮遗留的备忘与期望】\n")
	writeList(&b, j.Memos)
	b.WriteString("\n【上一轮查询到的参考资料】\n")
	writeList(&b, j.References)
	b.WriteString("\n【上一轮的错误与异常】\n")
	writeList(&b, j.Errors)
	b.WriteString("\n【历史操作的内容差异（最近若干条）】\n")
	b.WriteString(formatRecentOps(recent))
	if strings.TrimSpace(j.LastSummary) != "" {
		b.WriteString("\n【上一轮收尾】\n")
		b.WriteString(clipAtSentence(j.LastSummary, maxSummaryRunes))
		b.WriteByte('\n')
	}
	return b.String()
}

// formatRecentOps 格式化最近的内容级操作记录（只取上限内的条数，每条最多展示若干个差异块）。
func formatRecentOps(ops []fileflow.OpRecord) string {
	if len(ops) == 0 {
		return "（无）\n"
	}
	start := 0
	if len(ops) > maxContextOperations {
		start = len(ops) - maxContextOperations
	}
	return formatOpRecords(ops[start:])
}

// formatOpRecords 按插入顺序输出操作及其内容差异。
func formatOpRecords(ops []fileflow.OpRecord) string {
	var b strings.Builder
	for _, op := range ops {
		path := op.PathRel
		if path == "" {
			path = op.Target
		}
		fmt.Fprintf(&b, "#%d [%s] %s", op.Seq, op.Op, path)
		if op.ReviewStatus != "" && op.ReviewStatus != fileflow.ReviewPassed {
			b.WriteString(" · ")
			b.WriteString(clipAtSentence(op.ReviewDetail, maxErrorRunes))
		}
		if op.BeforeSHA256 != "" || op.AfterSHA256 != "" {
			fmt.Fprintf(&b, " · 哈希 %s → %s", shortHash(op.BeforeSHA256), shortHash(op.AfterSHA256))
		}
		b.WriteByte('\n')
		writeOpDeltas(&b, op.Deltas)
	}
	return b.String()
}

// writeOpDeltas 输出一条操作的内容差异（行号 / 旧片段 / 新片段 / 二进制结论）。
func writeOpDeltas(b *strings.Builder, deltas []fileflow.Delta) {
	if len(deltas) == 0 {
		b.WriteString("  （无内容增量）\n")
		return
	}
	shown := 0
	for _, delta := range deltas {
		if shown >= maxContextDeltas {
			b.WriteString("  …（其余差异块已省略）\n")
			break
		}
		shown++
		b.WriteString("  - ")
		switch {
		case delta.Kind == fileflow.DeltaRename:
			fmt.Fprintf(b, "重命名 %s → %s", delta.SourceRel, delta.PathRel)
		case delta.Binary:
			fmt.Fprintf(b, "%s%s｜%s", delta.PathRel, locatorText(delta), clipAtSentence(delta.BinaryNote, maxDeltaRunes))
		default:
			fmt.Fprintf(b, "%s%s｜-%s｜+%s", delta.PathRel, locatorText(delta),
				clipAtSentence(compactText(delta.OldText), maxDeltaRunes),
				clipAtSentence(compactText(delta.NewText), maxDeltaRunes))
		}
		if delta.Truncated {
			b.WriteString("（片段已截断）")
		}
		b.WriteByte('\n')
	}
}

// locatorText 输出定位前缀。
func locatorText(delta fileflow.Delta) string {
	if delta.Locator == "" {
		return ""
	}
	return "（" + delta.Locator + "）"
}

// compactText 把多行片段压成单行，便于提示词阅读。
func compactText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "⏎")
	text = strings.ReplaceAll(text, "\n", "⏎")
	if strings.TrimSpace(text) == "" {
		return "（空）"
	}
	return text
}

// shortHash 截短哈希，避免提示词被长哈希占满。
func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

// formatExpects 用一行概括规划预期。
func formatExpects(expects []fileflow.Expect) string {
	parts := make([]string, 0, len(expects))
	for _, expect := range expects {
		item := expect.Path
		if expect.Exists != nil {
			if *expect.Exists {
				item += " 存在"
			} else {
				item += " 不存在"
			}
		}
		if len(expect.Contains) > 0 {
			item += " 包含「" + previewRunes(strings.Join(expect.Contains, "、"), 40) + "」"
		}
		if len(expect.NotContains) > 0 {
			item += " 不含「" + previewRunes(strings.Join(expect.NotContains, "、"), 40) + "」"
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "；")
}

func writeList(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("（无）\n")
		return
	}
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(clipAtSentence(item, maxReferenceRunes))
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
