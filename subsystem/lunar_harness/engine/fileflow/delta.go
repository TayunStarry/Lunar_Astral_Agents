package fileflow

// delta.go 增量采集入口与文件状态

import (
	"LunarSubsystem/LunarHarness/engine/persistence"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
