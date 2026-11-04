package creation

// journal_change.go 内容级变更的提示词渲染：按操作分组、只注入最近若干次。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// formatChanges 按操作分组输出内容级变更，只注入最近 maxContextOperations 次操作。
func formatChanges(records []fileflow.ChangeRecord) string {
	if len(records) == 0 {
		return "（无）\n"
	}
	groups := groupChanges(records)
	if len(groups) > maxContextOperations {
		groups = groups[len(groups)-maxContextOperations:]
	}
	var b strings.Builder
	for _, group := range groups {
		fmt.Fprintf(&b, "#%d [%s] %s\n", group.seq, group.kind, group.path)
		writeChangeDeltas(&b, group.records)
	}
	return b.String()
}

// groupChanges 把同一操作的差异块聚合为组（按写入顺序）。
func groupChanges(records []fileflow.ChangeRecord) []changeGroup {
	groups := make([]changeGroup, 0)
	index := make(map[int]int)
	for _, record := range records {
		if position, ok := index[record.Seq]; ok {
			groups[position].records = append(groups[position].records, record)
			continue
		}
		index[record.Seq] = len(groups)
		groups = append(groups, changeGroup{
			seq:     record.Seq,
			kind:    record.Kind,
			path:    record.PathRel,
			records: []fileflow.ChangeRecord{record},
		})
	}
	return groups
}

// writeChangeDeltas 输出一组变更的内容差异（行号 / 旧片段 / 新片段 / 二进制结论）。
func writeChangeDeltas(b *strings.Builder, records []fileflow.ChangeRecord) {
	if len(records) == 0 {
		b.WriteString("  （无内容增量）\n")
		return
	}
	shown := 0
	for _, record := range records {
		if shown >= maxContextDeltas {
			b.WriteString("  …（其余差异块已省略）\n")
			break
		}
		shown++
		b.WriteString("  - ")
		switch {
		case record.Kind == fileflow.DeltaRename:
			fmt.Fprintf(b, "重命名 %s → %s", record.SourceRel, record.PathRel)
		case record.Binary:
			fmt.Fprintf(b, "%s%s｜%s", record.PathRel, changeLocator(record.Locator), clipAtSentence(record.BinaryNote, maxDeltaRunes))
		default:
			fmt.Fprintf(b, "%s%s｜-%s｜+%s", record.PathRel, changeLocator(record.Locator),
				clipAtSentence(compactText(record.OldText), maxDeltaRunes),
				clipAtSentence(compactText(record.NewText), maxDeltaRunes))
		}
		if record.Truncated {
			b.WriteString("（片段已截断）")
		}
		b.WriteByte('\n')
	}
}

// changeLocator 输出变更定位前缀。
func changeLocator(locator string) string {
	if locator == "" {
		return ""
	}
	return "（" + locator + "）"
}
