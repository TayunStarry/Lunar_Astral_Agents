package fileflow

// delta_entries.go 条目级增量

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func entryDeltas(input OpInput, entry Entry, before, after fileState) ([]Delta, error) {
	base := Delta{PathRel: entry.Rel, PathAbs: entry.Abs}
	archive := input.Op == OpArchiveWrite

	switch {
	case !before.Exists && !after.Exists:
		return nil, nil
	case !before.Exists && after.Exists:
		base.Kind = DeltaCreate
		if archive {
			base.Kind = DeltaArchive
		}
		return []Delta{wholeDelta(base, after, true, archive, input.Detail)}, nil
	case before.Exists && !after.Exists:
		base.Kind = DeltaDelete
		if archive {
			base.Kind = DeltaArchive
		}
		return []Delta{wholeDelta(base, before, false, archive, input.Detail)}, nil
	case before.IsDir && after.IsDir:
		base.Kind = DeltaDirBatch
		if archive {
			base.Kind = DeltaArchive
		}
		return []Delta{dirDelta(base, before, after, archive, input.Detail)}, nil
	case before.IsDir != after.IsDir:
		base.Kind = DeltaModify
		base.Binary = true
		base.BinaryNote = "文件与目录类型互换（整目标替换）"
		base.OldHash, base.NewHash = before.Hash, after.Hash
		base.OldSize, base.NewSize = before.Size, after.Size
		return []Delta{base}, nil
	}

	if archive || before.Binary() || after.Binary() {
		base.Kind = DeltaModify
		if archive {
			base.Kind = DeltaArchive
		}
		base.Binary = true
		base.OldHash, base.NewHash = before.Hash, after.Hash
		base.OldSize, base.NewSize = before.Size, after.Size
		base.BinaryNote = binaryChangeNote(input, before, after)
		return []Delta{base}, nil
	}

	base.Kind = DeltaModify
	base.OldHash, base.NewHash = before.Hash, after.Hash
	base.OldSize, base.NewSize = before.Size, after.Size
	hunks := textHunks(string(before.Data), string(after.Data))
	if len(hunks) == 0 {
		base.BinaryNote = "内容未变化"
		return []Delta{base}, nil
	}
	for index := range hunks {
		hunks[index].Kind = DeltaModify
		hunks[index].PathRel = entry.Rel
		hunks[index].PathAbs = entry.Abs
		hunks[index].OldHash, hunks[index].NewHash = before.Hash, after.Hash
		hunks[index].OldSize, hunks[index].NewSize = before.Size, after.Size
	}
	return hunks, nil
}

// wholeDelta 整文件新增 / 删除的增量。
func wholeDelta(base Delta, state fileState, added bool, archive bool, detail map[string]any) Delta {
	if added {
		base.OldSize, base.OldHash = 0, ""
		base.NewSize, base.NewHash = state.Size, state.Hash
	} else {
		base.OldSize, base.OldHash = state.Size, state.Hash
		base.NewSize, base.NewHash = 0, ""
	}
	if archive {
		base.Binary = true
		base.BinaryNote = archiveNote(detail, added)
		base.Locator = "压缩包整包写入"
		return base
	}
	if state.IsDir {
		base.Binary = true
		base.BinaryNote = fmt.Sprintf("目录级批量写：%d 个文件，共 %d 字节", len(state.Tree), state.Size)
		base.Locator = "目录整树写入"
		return base
	}
	if state.Binary() {
		base.Binary = true
		base.BinaryNote = fmt.Sprintf("整文件写入（二进制，%d 字节）", state.Size)
		base.Locator = "整文件"
		return base
	}
	text := string(state.Data)
	clipped, truncated := clipFragment(text)
	base.Truncated = truncated
	if added {
		base.NewText = clipped
		base.Locator = fmt.Sprintf("新增全文（%d 行 / %d 字节）", lineCount(text), state.Size)
	} else {
		base.OldText = clipped
		base.Locator = fmt.Sprintf("删除全文（%d 行 / %d 字节）", lineCount(text), state.Size)
	}
	return base
}

// dirDelta 目录级批量写：比较前后文件清单，给出新增 / 删除 / 修改的数量与代表条目。
func dirDelta(base Delta, before, after fileState, archive bool, detail map[string]any) Delta {
	added := make([]string, 0)
	removed := make([]string, 0)
	modified := make([]string, 0)
	for rel, file := range after.Tree {
		old, ok := before.Tree[rel]
		switch {
		case !ok:
			added = append(added, rel)
		case old.SHA256 != file.SHA256 || old.Size != file.Size:
			modified = append(modified, rel)
		}
	}
	for rel := range before.Tree {
		if _, ok := after.Tree[rel]; !ok {
			removed = append(removed, rel)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(modified)
	base.OldSize, base.NewSize = before.Size, after.Size
	note := fmt.Sprintf("目录批量写：新增 %d、删除 %d、修改 %d（共 %d 个文件，%d → %d 字节）",
		len(added), len(removed), len(modified), len(after.Tree), before.Size, after.Size)
	base.BinaryNote = appendItems(note, added, removed, modified)
	if archive {
		base.Kind = DeltaArchive
		base.BinaryNote = archiveNote(detail, true) + "；" + base.BinaryNote
	}
	base.Binary = true
	base.Truncated = len(added)+len(removed)+len(modified) > maxDeltaTreeItems
	base.Locator = "目录整树"
	return base
}

// appendItems 把变更清单接在结论后（超出上限时截断）。
func appendItems(note string, lists ...[]string) string {
	var b strings.Builder
	b.WriteString(note)
	total := 0
	for _, list := range lists {
		for _, item := range list {
			if total >= maxDeltaTreeItems {
				b.WriteString("；…（其余条目已省略）")
				return b.String()
			}
			b.WriteString("；")
			b.WriteString(item)
			total++
		}
	}
	return b.String()
}

// binaryChangeNote 生成二进制文件的可读差异结论。
func binaryChangeNote(input OpInput, before, after fileState) string {
	if input.Op == OpArchiveWrite {
		return archiveNote(input.Detail, !before.Exists)
	}
	if before.Hash != "" && before.Hash == after.Hash {
		return "内容未变化"
	}
	switch {
	case before.Size == 0 && after.Size > 0:
		return fmt.Sprintf("整文件写入（%d 字节）", after.Size)
	case after.Size > before.Size && len(after.Data) > 0 && bytes.HasPrefix(after.Data, before.Data):
		added := after.Size - before.Size
		return fmt.Sprintf("末尾追加 %d 字节（%d 块，块大小 4KB）", added, blockCount(added))
	case after.Size < before.Size && len(after.Data) > 0 && bytes.HasPrefix(before.Data, after.Data):
		return fmt.Sprintf("末尾截断 %d 字节（剩余 %d 字节）", before.Size-after.Size, after.Size)
	}
	return fmt.Sprintf("整文件替换（%d → %d 字节，%d 块中 %d 块不同）",
		before.Size, after.Size, blockCount(maxInt64(before.Size, after.Size)), changedBlocks(before.Data, after.Data))
}

// archiveNote 压缩包写入的可读结论。
func archiveNote(detail map[string]any, created bool) string {
	action := ""
	if detail != nil {
		if value, ok := detail["action"].(string); ok {
			action = strings.TrimSpace(value)
		}
		if value, ok := detail["entry"].(string); ok && strings.TrimSpace(value) != "" {
			action += "（条目 " + strings.TrimSpace(value) + "）"
		}
	}
	if action == "" {
		if created {
			action = "创建压缩包"
		} else {
			action = "改写压缩包"
		}
	}
	return "压缩包写入：" + action
}

// textHunks 计算文本的行级差异块（前缀 / 后缀对齐 + 有限前瞻同步点）。
