package fileflow

// delta_text.go 文本行块差分

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

func textHunks(oldText, newText string) []Delta {
	if oldText == newText {
		return nil
	}
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	oldSeg := oldLines[prefix : len(oldLines)-suffix]
	newSeg := newLines[prefix : len(newLines)-suffix]
	return groupHunks(oldSeg, newSeg, prefix)
}

// groupHunks 把差异段切成若干块；块数超限时用一条 truncated 记录兜底（回退改用快照）。
func groupHunks(oldSeg, newSeg []string, base int) []Delta {
	out := make([]Delta, 0)
	i, j := 0, 0
	oldStart, newStart := 0, 0
	flush := func() bool {
		if i == oldStart && j == newStart {
			return true
		}
		out = append(out, hunkDelta(oldSeg[oldStart:i], newSeg[newStart:j], base+newStart, base+oldStart))
		oldStart, newStart = i, j
		if len(out) >= maxDeltaHunks {
			out = append(out, Delta{
				Kind: DeltaModify, Locator: "差异块数超过上限",
				BinaryNote: fmt.Sprintf("差异块数超过上限 %d，其余变更已省略", maxDeltaHunks),
				Truncated:  true,
			})
			return false
		}
		return true
	}
	for i < len(oldSeg) || j < len(newSeg) {
		if i < len(oldSeg) && j < len(newSeg) && oldSeg[i] == newSeg[j] {
			if !flush() {
				return out
			}
			i++
			j++
			oldStart, newStart = i, j
			continue
		}
		a, b := lookaheadSync(oldSeg, newSeg, i, j)
		if a >= 0 && b >= 0 && (a > 0 || b > 0) {
			i += a
			j += b
			continue
		}
		switch {
		case i < len(oldSeg) && j < len(newSeg):
			i++
			j++
		case i < len(oldSeg):
			i++
		default:
			j++
		}
	}
	flush()
	return out
}

// lookaheadSync 在前瞻窗口内寻找下一个可对齐的相同行（允许单侧插入 / 删除）。
func lookaheadSync(oldSeg, newSeg []string, i, j int) (int, int) {
	const window = 8
	bestA, bestB, bestSum := -1, -1, 1<<30
	for a := 0; a <= window; a++ {
		if i+a >= len(oldSeg) {
			break
		}
		for b := 0; b <= window; b++ {
			if j+b >= len(newSeg) {
				break
			}
			if a == 0 && b == 0 {
				continue
			}
			if oldSeg[i+a] != newSeg[j+b] {
				continue
			}
			if a+b < bestSum {
				bestSum, bestA, bestB = a+b, a, b
			}
		}
	}
	return bestA, bestB
}

// hunkDelta 构造一个文本差异块，行号以操作后的文件为准。
func hunkDelta(oldSeg, newSeg []string, newStart, oldStart int) Delta {
	oldText := strings.Join(oldSeg, "\n")
	newText := strings.Join(newSeg, "\n")
	clippedOld, oldCut := clipFragment(oldText)
	clippedNew, newCut := clipFragment(newText)
	delta := Delta{
		Kind:      DeltaModify,
		OldText:   clippedOld,
		NewText:   clippedNew,
		Truncated: oldCut || newCut,
		LineFrom:  newStart + 1,
		LineTo:    newStart + len(newSeg),
	}
	switch {
	case len(newSeg) == 0:
		delta.Locator = fmt.Sprintf("删除 原第 %d-%d 行", oldStart+1, oldStart+len(oldSeg))
	case len(oldSeg) == 0:
		delta.Locator = fmt.Sprintf("新增 新第 %d-%d 行", newStart+1, newStart+len(newSeg))
	default:
		delta.Locator = fmt.Sprintf("替换 原第 %d-%d 行 → 新第 %d-%d 行",
			oldStart+1, oldStart+len(oldSeg), newStart+1, newStart+len(newSeg))
	}
	return delta
}

// clipFragment 按字节上限截断片段，保留合法 UTF-8 边界。
func clipFragment(text string) (string, bool) {
	if len(text) <= maxDeltaFragmentBytes {
		return text, false
	}
	cut := text[:maxDeltaFragmentBytes]
	for len(cut) > 0 {
		if utf8.ValidString(cut) {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut + "…（片段已截断）", true
}

// splitLines 按行拆分，保留末尾空行（文件以换行结尾时最后一项为空串）。
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// lineCount 统计行数（末尾换行不算额外一行）。
func lineCount(text string) int {
	if text == "" {
		return 0
	}
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return 1
	}
	return strings.Count(trimmed, "\n") + 1
}

// isTextData 判断内容是否按文本处理（合法 UTF-8 且不含 NUL）。
func isTextData(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

// Binary 判断状态是否按二进制处理（超限或内容不是文本）。
func (s fileState) Binary() bool {
	if s.IsDir {
		return false
	}
	if len(s.Data) == 0 && s.Size > 0 {
		return true
	}
	return !s.IsText
}

// blockCount 计算 4KB 块数（至少 1）。
func blockCount(size int64) int64 {
	blocks := size / (4 << 10)
	if size%(4<<10) != 0 {
		blocks++
	}
	if blocks == 0 {
		blocks = 1
	}
	return blocks
}

// changedBlocks 统计前后内容在 4KB 块上的差异数量。
func changedBlocks(before, after []byte) int64 {
	const block = 4 << 10
	total := maxInt64(int64(len(before)), int64(len(after)))
	changed := int64(0)
	for offset := int64(0); offset < total; offset += block {
		window := func(data []byte) []byte {
			if offset >= int64(len(data)) {
				return nil
			}
			end := offset + block
			if end > int64(len(data)) {
				end = int64(len(data))
			}
			return data[offset:end]
		}
		if !bytes.Equal(window(before), window(after)) {
			changed++
		}
	}
	return changed
}

// maxInt64 取较大值。
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
