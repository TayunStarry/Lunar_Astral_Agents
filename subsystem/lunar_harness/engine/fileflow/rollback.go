package fileflow

// rollback.go 按流程回退到指定步骤：以数据库内容增量为权威差异反向还原，
// 文本差异完整时只靠数据库即可重建；二进制 / 目录 / 片段被截断时改用该步骤的快照兜底。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// RollbackTo 回退流程中 seq > targetSeq 的全部操作（按序号倒序）。
func RollbackTo(flowID string, targetSeq int) (RollbackResult, error) {
	result := RollbackResult{FlowID: flowID, TargetSeq: targetSeq}
	ops, err := FlowOps(flowID)
	if err != nil {
		return result, err
	}
	pending := make([]OpRecord, 0, len(ops))
	maxSeq := targetSeq
	for _, op := range ops {
		if op.Seq > maxSeq {
			maxSeq = op.Seq
		}
		if op.Seq > targetSeq && op.Status != OpStatusRolledBack {
			pending = append(pending, op)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Seq > pending[j].Seq })

	for _, op := range pending {
		if err := reverseOp(op); err != nil {
			_ = markOpStatus(op.ID, OpStatusRollbackFailed, false, err.Error())
			result.Errors = append(result.Errors, fmt.Sprintf("步骤 %d %s：%v", op.Seq, op.Op, err))
			continue
		}
		_ = markOpStatus(op.ID, OpStatusRolledBack, true, "")
		result.Reverted++
	}
	result.OK = len(result.Errors) == 0
	insertRollback(flowID, targetSeq, maxSeq, result.OK, rollbackDetail(result))
	if result.OK {
		resourcehub.Invalidate()
	}
	return result, nil
}

// rollbackDetail 拼装回退记录的可读描述。
func rollbackDetail(result RollbackResult) string {
	if result.OK {
		return fmt.Sprintf("回退到步骤 %d，还原 %d 条操作", result.TargetSeq, result.Reverted)
	}
	return strings.Join(result.Errors, "；")
}

// reverseOp 反向还原一条操作：优先用内容增量，必要时用快照。
func reverseOp(op OpRecord) error {
	needsSnapshot := false
	for _, delta := range op.Deltas {
		if delta.Truncated || delta.Kind == DeltaDirBatch || delta.Kind == DeltaArchive || (delta.Binary && delta.Kind != DeltaRename) {
			needsSnapshot = true
			break
		}
	}
	if needsSnapshot || len(op.Deltas) == 0 {
		return restoreFromSnapshot(op)
	}
	for _, delta := range op.Deltas {
		if err := reverseDelta(delta); err != nil {
			return err
		}
	}
	return verifyRestored(op.Deltas)
}

// restoreFromSnapshot 使用该步骤的操作前快照兜底还原。
func restoreFromSnapshot(op OpRecord) error {
	if strings.TrimSpace(op.Snapshot) == "" {
		return fmt.Errorf("缺少快照目录，且内容增量不足以还原")
	}
	snapshot, err := loadSnapshot(op.Snapshot)
	if err != nil {
		return err
	}
	if err := Restore(snapshot); err != nil {
		return fmt.Errorf("快照还原失败: %w", err)
	}
	return nil
}

// loadSnapshot 从流程目录读取快照清单。
func loadSnapshot(dirRel string) (Snapshot, error) {
	raw, ok := persistence.ReadFileBytes(filepath.ToSlash(filepath.Join(dirRel, manifestFile)))
	if !ok {
		return Snapshot{}, fmt.Errorf("读取快照清单失败: %s", dirRel)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("解析快照清单失败: %w", err)
	}
	if snapshot.Dir == "" {
		snapshot.Dir = dirRel
	}
	return snapshot, nil
}

// reverseDelta 反向应用一条内容增量。
func reverseDelta(delta Delta) error {
	switch delta.Kind {
	case DeltaCreate:
		if delta.PathAbs == "" {
			return fmt.Errorf("新增记录缺少目标路径")
		}
		if err := os.RemoveAll(delta.PathAbs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("撤销新增失败 %s: %w", delta.PathAbs, err)
		}
		return nil
	case DeltaDelete:
		if delta.OldText == "" && delta.OldSize > 0 {
			return fmt.Errorf("删除记录缺少可还原内容（需快照兜底）")
		}
		if err := writeText(delta.PathRel, delta.PathAbs, delta.OldText); err != nil {
			return err
		}
		return nil
	case DeltaRename:
		if delta.SourceRel == "" || delta.PathAbs == "" {
			return fmt.Errorf("重命名记录缺少来源或目标")
		}
		source := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(delta.SourceRel))
		if _, err := os.Stat(delta.PathAbs); err != nil {
			return fmt.Errorf("重命名目标不存在，无法还原 %s: %w", delta.PathAbs, err)
		}
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			return fmt.Errorf("创建来源目录失败: %w", err)
		}
		if err := os.Rename(delta.PathAbs, source); err != nil {
			return fmt.Errorf("还原重命名失败 %s → %s: %w", delta.PathAbs, source, err)
		}
		return nil
	}
	return reverseHunks(delta)
}

// reverseHunks 把一个修改块反向写回文件（行号以操作后文件为基准）。
func reverseHunks(delta Delta) error {
	abs := delta.PathAbs
	rel := delta.PathRel
	if abs == "" && rel != "" {
		abs = filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	}
	if abs == "" {
		return fmt.Errorf("修改记录缺少目标路径")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("读取待还原文件失败 %s: %w", abs, err)
	}
	lines := strings.Split(string(data), "\n")
	start := delta.LineFrom - 1
	end := delta.LineTo
	if end < start {
		end = start
	}
	if start < 0 || start > len(lines) || end > len(lines) {
		return fmt.Errorf("内容增量行号越界（文件已变化）: %s", abs)
	}
	replacement := []string{}
	if delta.OldText != "" {
		replacement = strings.Split(delta.OldText, "\n")
	}
	merged := make([]string, 0, len(lines)+len(replacement))
	merged = append(merged, lines[:start]...)
	merged = append(merged, replacement...)
	merged = append(merged, lines[end:]...)
	return writeText(rel, abs, strings.Join(merged, "\n"))
}

// writeText 原子写回文本（相对路径优先，保证进入资源索引目录）。
func writeText(rel, abs, text string) error {
	if rel == "" && abs != "" {
		if relPath, err := filepath.Rel(filepath.Clean(config.LocalDir), abs); err == nil {
			rel = filepath.ToSlash(relPath)
		}
	}
	if rel != "" {
		if err := persistence.WriteFileAtomic(rel, []byte(text)); err != nil {
			return fmt.Errorf("写回文件失败 %s: %w", rel, err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.WriteFile(abs, []byte(text), 0o644); err != nil {
		return fmt.Errorf("写回文件失败 %s: %w", abs, err)
	}
	return nil
}

// verifyRestored 校验还原后的内容哈希是否等于记录中的操作前哈希。
func verifyRestored(deltas []Delta) error {
	for _, delta := range deltas {
		if delta.OldHash == "" || delta.Binary || delta.Kind == DeltaCreate || delta.Kind == DeltaRename {
			continue
		}
		state, err := readState(delta.PathAbs)
		if err != nil {
			return err
		}
		if !state.Exists || state.Hash != delta.OldHash {
			return fmt.Errorf("还原后内容哈希不一致: %s", delta.PathRel)
		}
	}
	return nil
}
