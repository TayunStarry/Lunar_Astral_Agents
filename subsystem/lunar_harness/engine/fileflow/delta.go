package fileflow

// delta.go 内容级增量的生成：操作前状态来自快照，操作后状态来自真实文件系统；
// 文本按行块记录「替换前 / 替换后 + 行号」，二进制只记录哈希、大小与可读差异结论。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/persistence"
)

// BuildOp 采集操作后的内容增量（必须在审查 / 回滚之前调用，否则会采到已还原的状态）。
func BuildOp(input OpInput) (*BuiltOp, error) {
	if strings.TrimSpace(input.FlowID) == "" {
		return nil, fmt.Errorf("缺少流程 ID")
	}
	deltas, beforeHash, afterHash, beforeSize, afterSize, err := buildDeltas(input)
	if err != nil {
		return nil, err
	}
	return &BuiltOp{
		Input: input, Deltas: deltas,
		BeforeHash: beforeHash, AfterHash: afterHash,
		BeforeSize: beforeSize, AfterSize: afterSize,
	}, nil
}

// Commit 写入操作记录与内容增量，并附带最终审查结论。
func (b *BuiltOp) Commit(review Review) error {
	if b == nil {
		return fmt.Errorf("缺少已采集的操作增量")
	}
	b.Input.Review = review
	return insertOp(b.Input, b.BeforeHash, b.AfterHash, b.BeforeSize, b.AfterSize, b.Deltas)
}

// buildDeltas 按快照条目逐个比对操作前 / 操作后状态，并汇总主目标的哈希与大小。
func buildDeltas(input OpInput) ([]Delta, string, string, int64, int64, error) {
	entries := input.Snapshot.Entries
	if len(entries) == 0 {
		return nil, "", "", 0, 0, fmt.Errorf("操作缺少快照条目，无法生成内容增量")
	}

	deltas := make([]Delta, 0, len(entries))
	var beforeHash, afterHash string
	var beforeSize, afterSize int64
	primarySet := false
	skip := map[string]bool{}

	// 移动 / 重命名：来源消失 + 目标出现，合并为一条 rename 增量，避免拆成删除 + 新增。
	if input.Op == OpMove && len(entries) >= 2 {
		source, target, ok := detectRename(entries)
		if ok {
			delta := Delta{
				Kind:      DeltaRename,
				PathRel:   target.Rel,
				PathAbs:   target.Abs,
				SourceRel: source.Rel,
				OldHash:   source.SHA256,
				OldSize:   source.Size,
				Locator:   fmt.Sprintf("重命名 / 移动：%s → %s", source.Rel, target.Rel),
			}
			if after, err := readState(target.Abs); err == nil {
				delta.NewHash = after.Hash
				delta.NewSize = after.Size
			}
			deltas = append(deltas, delta)
			beforeHash, beforeSize, afterHash, afterSize = source.SHA256, source.Size, delta.NewHash, delta.NewSize
			primarySet = true
			skip[source.Abs] = true
			skip[target.Abs] = true
		}
	}

	for _, entry := range entries {
		if skip[entry.Abs] {
			continue
		}
		before := beforeState(entry)
		after, err := readState(entry.Abs)
		if err != nil {
			return nil, "", "", 0, 0, err
		}
		if !primarySet && strings.TrimSpace(input.PathAbs) != "" && filepath.Clean(entry.Abs) == filepath.Clean(input.PathAbs) {
			beforeHash, beforeSize = before.Hash, before.Size
			afterHash, afterSize = after.Hash, after.Size
			primarySet = true
		}
		items, itemErr := entryDeltas(input, entry, before, after)
		if itemErr != nil {
			return nil, "", "", 0, 0, itemErr
		}
		deltas = append(deltas, items...)
	}
	if !primarySet && len(deltas) > 0 {
		beforeHash, beforeSize = deltas[0].OldHash, deltas[0].OldSize
		afterHash, afterSize = deltas[0].NewHash, deltas[0].NewSize
	}
	return deltas, beforeHash, afterHash, beforeSize, afterSize, nil
}

// detectRename 在移动快照里识别「来源消失、目标出现」的一组条目。
func detectRename(entries []Entry) (source, target Entry, ok bool) {
	for _, entry := range entries {
		if !entry.Existed {
			continue
		}
		state, err := readState(entry.Abs)
		if err != nil || state.Exists {
			continue
		}
		for _, candidate := range entries {
			if candidate.Existed || filepath.Clean(candidate.Abs) == filepath.Clean(entry.Abs) {
				continue
			}
			if state, err := readState(candidate.Abs); err == nil && state.Exists {
				return entry, candidate, true
			}
		}
	}
	return Entry{}, Entry{}, false
}

// beforeState 由快照条目重建操作前状态。
func beforeState(entry Entry) fileState {
	if !entry.Existed {
		return fileState{}
	}
	if entry.IsDir {
		tree := make(map[string]TreeFile, len(entry.Files))
		for _, file := range entry.Files {
			tree[file.Rel] = file
		}
		return fileState{Exists: true, IsDir: true, Size: entry.Size, Tree: tree}
	}
	data, ok := persistence.ReadFileBytes(entry.Blob)
	if !ok {
		return fileState{Exists: true, Size: entry.Size, Hash: entry.SHA256}
	}
	return fileState{
		Exists: true, Size: entry.Size, Hash: entry.SHA256,
		Data: data, IsText: isTextData(data),
	}
}

// readState 读取路径当前状态；超过读取上限的文件只计算流式哈希。
func readState(abs string) (fileState, error) {
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return fileState{}, nil
	}
	if err != nil {
		return fileState{}, fmt.Errorf("读取操作后状态失败 %s: %w", abs, err)
	}
	if info.IsDir() {
		tree, total, walkErr := dirTree(abs)
		if walkErr != nil {
			return fileState{}, walkErr
		}
		return fileState{Exists: true, IsDir: true, Size: total, Tree: tree}, nil
	}
	if info.Size() <= maxDeltaReadBytes {
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			return fileState{}, fmt.Errorf("读取操作后内容失败 %s: %w", abs, readErr)
		}
		return fileState{
			Exists: true, Size: info.Size(), Hash: hashBytes(data),
			Data: data, IsText: isTextData(data),
		}, nil
	}
	hash, hashErr := hashFile(abs)
	if hashErr != nil {
		return fileState{}, hashErr
	}
	return fileState{Exists: true, Size: info.Size(), Hash: hash}, nil
}

// dirTree 遍历目录，返回「相对路径 → 文件信息」与总字节数。
func dirTree(abs string) (map[string]TreeFile, int64, error) {
	tree := make(map[string]TreeFile)
	var total int64
	err := filepath.WalkDir(abs, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		hash := ""
		if info.Size() <= maxDeltaReadBytes {
			if data, readErr := os.ReadFile(path); readErr == nil {
				hash = hashBytes(data)
			}
		}
		total += info.Size()
		tree[filepath.ToSlash(rel)] = TreeFile{Rel: filepath.ToSlash(rel), Size: info.Size(), SHA256: hash}
		return nil
	})
	return tree, total, err
}

// hashFile 流式计算文件哈希。
func hashFile(abs string) (string, error) {
	handle, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("打开文件失败 %s: %w", abs, err)
	}
	defer handle.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, handle); err != nil {
		return "", fmt.Errorf("计算文件哈希失败 %s: %w", abs, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// entryDeltas 生成单个快照路径的内容增量。
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
