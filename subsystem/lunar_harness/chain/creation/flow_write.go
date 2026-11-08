package creation

// flow_write.go 写入类操作节点：文本文件与压缩包的全部写操作。
// 每个写操作统一经 mutate 完成「路由 → 快照 → 执行 → 审查 → 回滚 → 记录」。

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
)

// fileCreate 新建文件。
func (s *session) fileCreate(args map[string]any) toolOutcome {
	target := argString(args, "path")
	content := argString(args, "content")
	if strings.HasSuffix(strings.TrimSpace(target), "/") || strings.HasSuffix(strings.TrimSpace(target), "\\") {
		return failure(toolFileCreate, fmt.Errorf("file_create 的 path 必须带具体文件名，不能以分隔符结尾（目录不会单独创建，父目录会随文件自动创建）"))
	}
	if !argBool(args, "allow_duplicate") {
		if warning := duplicateNameWarning(target); warning != "" {
			return failure(toolFileCreate, fmt.Errorf("%s", warning))
		}
	}
	return s.mutate(fileflow.OpCreate, toolFileCreate, target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult { return fromFileEditor(FileEditor.CreateFile(target, content)) })
}

// fileRewrite 整篇写入。
func (s *session) fileRewrite(args map[string]any) toolOutcome {
	target := argString(args, "path")
	content := argString(args, "content")
	if !argBool(args, "allow_duplicate") {
		if warning := duplicateNameWarning(target); warning != "" {
			return failure(toolFileRewrite, fmt.Errorf("%s", warning))
		}
	}
	return s.mutate(fileflow.OpWrite, toolFileRewrite, target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult { return fromFileEditor(FileEditor.WriteFile(target, content)) })
}

// duplicateNameWarning 新建前的同名检测：同名文件已在其它目录时返回带候选路径的提示。
//
// 目标已存在时交由「文件已存在」的既有逻辑处理；这里只拦「换个目录再建一份同名文件」，
// 这是用户最初「文件被覆盖 / 目录混乱」痛点的硬性保证。
func duplicateNameWarning(target string) string {
	abs, rel, err := fileflow.Resolve(target)
	if err != nil {
		return ""
	}
	if _, statErr := os.Stat(abs); statErr == nil {
		return ""
	}
	base := path.Base(rel)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	candidates := sameNameFiles(base, rel)
	if len(candidates) == 0 {
		return ""
	}
	return fmt.Sprintf("已存在同名文件：%s。请直接复用已有路径（file_rewrite / file_replace），不要换目录新建；确需新建到 %s，请传 allow_duplicate=true",
		strings.Join(candidates, "、"), rel)
}

// sameNameFiles 在 documents / unknown 下按文件名查找候选（不读正文，最多 maxDuplicateCandidates 条）。
func sameNameFiles(base, excludeRel string) []string {
	found := make([]string, 0, maxDuplicateCandidates)
	for _, root := range pathSnapshotRoots {
		start := filepath.Join(config.LocalDir, filepath.FromSlash(root))
		_ = filepath.WalkDir(start, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry == nil || entry.IsDir() {
				return nil
			}
			if !strings.EqualFold(entry.Name(), base) {
				return nil
			}
			rel, relErr := filepath.Rel(config.LocalDir, current)
			if relErr != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if relSlash == excludeRel {
				return nil
			}
			found = append(found, relSlash)
			if len(found) >= maxDuplicateCandidates {
				return filepath.SkipAll
			}
			return nil
		})
		if len(found) >= maxDuplicateCandidates {
			break
		}
	}
	return found
}

// fileRemove 清空文件内容（保留文件）。
func (s *session) fileRemove(args map[string]any) toolOutcome {
	target := argString(args, "path")
	return s.mutate(fileflow.OpWrite, toolFileRemove, target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult { return fromFileEditor(FileEditor.WriteFile(target, "")) })
}

// fileDelete 删除文件。
func (s *session) fileDelete(args map[string]any) toolOutcome {
	target := argString(args, "path")
	return s.mutate(fileflow.OpDelete, toolFileDelete, target, []string{target},
		[]fileflow.Expect{expectExists(target, false)},
		func() mutationResult { return fromFileEditor(FileEditor.DeleteFile(target)) })
}

// fileMove 移动 / 重命名。
func (s *session) fileMove(args map[string]any) toolOutcome {
	source := argString(args, "path")
	target := argString(args, "target")
	return s.mutate(fileflow.OpMove, toolFileMove, target, []string{source, target},
		[]fileflow.Expect{expectExists(target, true), expectExists(source, false)},
		func() mutationResult { return fromFileEditor(FileEditor.MoveFile(source, target)) })
}

// fileCopy 复制文件。
func (s *session) fileCopy(args map[string]any) toolOutcome {
	source := argString(args, "path")
	target := argString(args, "target")
	return s.mutate(fileflow.OpCopy, "file_copy", target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult { return fromFileEditor(FileEditor.CopyFile(source, target)) })
}

// fileReplace 按原文替换（可全部替换）。
func (s *session) fileReplace(args map[string]any) toolOutcome {
	target := argString(args, "path")
	search := argString(args, "search")
	replace := argString(args, "replace")
	if strings.TrimSpace(search) == "" {
		return failure(toolFileReplace, fmt.Errorf("要查找的原文不能为空"))
	}
	return s.mutate(fileflow.OpReplace, toolFileReplace, target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult {
			content, err := FileEditor.ReadText(target)
			if err != nil {
				return mutationResult{Summary: err.Error()}
			}
			if !strings.Contains(content, search) {
				return mutationResult{Summary: "未找到要替换的内容"}
			}
			next := content
			if argBool(args, "all") {
				next = strings.ReplaceAll(content, search, replace)
			} else {
				next = strings.Replace(content, search, replace, 1)
			}
			return fromFileEditor(FileEditor.WriteFile(target, next))
		})
}

// fileDeleteText 删除指定原文片段。
func (s *session) fileDeleteText(args map[string]any) toolOutcome {
	target := argString(args, "path")
	text := argString(args, "text")
	if strings.TrimSpace(text) == "" {
		return failure(toolFileDeleteText, fmt.Errorf("要删除的原文不能为空"))
	}
	return s.mutate(fileflow.OpReplace, toolFileDeleteText, target, []string{target},
		[]fileflow.Expect{expectExists(target, true)},
		func() mutationResult {
			content, err := FileEditor.ReadText(target)
			if err != nil {
				return mutationResult{Summary: err.Error()}
			}
			if !strings.Contains(content, text) {
				return mutationResult{Summary: "未找到要删除的内容"}
			}
			next := strings.Replace(content, text, "", 1)
			if argBool(args, "all") {
				next = strings.ReplaceAll(content, text, "")
			}
			return fromFileEditor(FileEditor.WriteFile(target, next))
		})
}

// archiveWrite 压缩包写入：创建 / 删除 / 新增条目 / 移除条目。
func (s *session) archiveWrite(args map[string]any) toolOutcome {
	action := strings.ToLower(strings.TrimSpace(argString(args, "action")))
	target := strings.TrimSpace(argString(args, "target"))
	if target == "" {
		return failure(toolArchiveWrite, fmt.Errorf("target 不能为空"))
	}
	switch action {
	case "create":
		sources := argStrings(args, "sources")
		outputRel, err := ArchiveOps.OutputPath(target)
		if err != nil {
			return failure(toolArchiveWrite, err)
		}
		expects := remapExpects(s.stepExpects(), target, outputRel)
		expects = append(expects, expectExists(outputRel, true))
		return s.mutate(fileflow.OpArchiveWrite, toolArchiveWrite, target, []string{outputRel}, expects,
			func() mutationResult { return fromArchive(ArchiveOps.Create(target, sources)) })
	case "delete":
		_, rel, err := ArchiveReader.ResolveArchivePath(target)
		if err != nil {
			return failure(toolArchiveWrite, err)
		}
		expects := remapExpects(s.stepExpects(), target, rel)
		expects = append(expects, expectExists(rel, false))
		return s.mutate(fileflow.OpArchiveWrite, toolArchiveWrite, target, []string{rel}, expects,
			func() mutationResult { return fromArchive(ArchiveOps.Delete(target)) })
	case "add":
		_, rel, err := ArchiveReader.ResolveArchivePath(target)
		if err != nil {
			return failure(toolArchiveWrite, err)
		}
		expects := remapExpects(s.stepExpects(), target, rel)
		expects = append(expects, expectExists(rel, true))
		return s.mutate(fileflow.OpArchiveWrite, toolArchiveWrite, target, []string{rel}, expects,
			func() mutationResult {
				return fromArchive(ArchiveOps.AddEntry(target,
					argString(args, "entry"), argString(args, "source"), argString(args, "content")))
			})
	case "remove":
		_, rel, err := ArchiveReader.ResolveArchivePath(target)
		if err != nil {
			return failure(toolArchiveWrite, err)
		}
		expects := remapExpects(s.stepExpects(), target, rel)
		expects = append(expects, expectExists(rel, true))
		return s.mutate(fileflow.OpArchiveWrite, toolArchiveWrite, target, []string{rel}, expects,
			func() mutationResult {
				return fromArchive(ArchiveOps.RemoveEntry(target, argString(args, "entry")))
			})
	default:
		return failure(toolArchiveWrite, fmt.Errorf("action 只能取 create / delete / add / remove"))
	}
}
