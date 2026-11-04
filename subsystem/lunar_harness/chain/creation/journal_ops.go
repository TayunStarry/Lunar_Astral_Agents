package creation

// journal_ops.go 操作记录与通用文本片段的提示词渲染。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

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
			item += " 包含「" + previewRunes(strings.Join(expect.Contains, "、"), 512) + "」"
		}
		if len(expect.NotContains) > 0 {
			item += " 不含「" + previewRunes(strings.Join(expect.NotContains, "、"), 512) + "」"
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
