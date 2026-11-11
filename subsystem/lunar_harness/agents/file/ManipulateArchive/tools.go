package ManipulateArchive

// tools.go 归档操纵工具：创建、删除、列出、提取与追加压缩包条目。

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/agents/file/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/file/filesupport"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
)

const (
	toolCreate    = "archive_create"
	toolUninstall = "archive_uninstall"
	toolExtract   = "archive_extract"
	toolList      = "archive_list"
	toolAdd       = "archive_add"
)

type collector struct {
	root          string
	handle        []string
	changedPaths  []string
	evidenceItems []string
}

func (c *collector) addHandle(handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || contains(c.handle, handle) {
			continue
		}
		c.handle = append(c.handle, handle)
	}
}

func (c *collector) addChanged(rel string) {
	if rel == "" || contains(c.changedPaths, rel) {
		return
	}
	c.changedPaths = append(c.changedPaths, rel)
}

func (c *collector) addEvidence(text string) {
	text = strings.TrimSpace(text)
	if text == "" || len(c.evidenceItems) >= 8 {
		return
	}
	c.evidenceItems = append(c.evidenceItems, clip(text, 1200))
}

func (c *collector) handles() []string  { return append([]string(nil), c.handle...) }
func (c *collector) changed() []string  { return append([]string(nil), c.changedPaths...) }
func (c *collector) evidence() []string { return append([]string(nil), c.evidenceItems...) }

func toolHandler(c *collector) file.Handler {
	return func(name string, args map[string]any) (file.Outcome, bool, error) {
		switch name {
		case toolCreate:
			return archiveCreate(args, c), false, nil
		case toolUninstall:
			return archiveUninstall(args, c), false, nil
		case toolExtract:
			return archiveExtract(args, c), false, nil
		case toolList:
			return archiveList(args), false, nil
		case toolAdd:
			return archiveAdd(args, c), false, nil
		default:
			return file.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func archiveCreate(args map[string]any, c *collector) file.Outcome {
	target := file.StringArg(args, "target")
	sources := file.StringSliceArg(args, "sources")
	if target == "" {
		return file.Failure(toolCreate, fmt.Errorf("target 不能为空"))
	}
	if len(sources) == 0 {
		return file.Failure(toolCreate, fmt.Errorf("sources 不能为空"))
	}
	actualRel, err := ArchiveOps.OutputPath(target)
	if err != nil {
		return file.Failure(toolCreate, err)
	}
	if _, err := filesupport.BackupWithOp(actualRel, "archive_create"); err != nil {
		return file.Failure(toolCreate, fmt.Errorf("保存版本失败：%w", err))
	}
	relSources := make([]string, 0, len(sources))
	for _, source := range sources {
		rel, _, resolveErr := filesupport.Resolve(source, c.root)
		if resolveErr != nil {
			return file.Failure(toolCreate, resolveErr)
		}
		relSources = append(relSources, rel)
	}
	result := ArchiveOps.Create(target, relSources)
	return mutated(c, actualRel, result, "创建归档 "+actualRel+"，来源："+strings.Join(relSources, "、"))
}

func archiveUninstall(args map[string]any, c *collector) file.Outcome {
	target := file.StringArg(args, "target")
	rel, err := resolveArchive(target)
	if err != nil {
		return file.Failure(toolUninstall, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "archive_uninstall"); err != nil {
		return file.Failure(toolUninstall, fmt.Errorf("保存版本失败：%w", err))
	}
	result := ArchiveOps.Delete(target)
	return mutated(c, rel, result, "删除归档 "+rel)
}

func archiveAdd(args map[string]any, c *collector) file.Outcome {
	target := file.StringArg(args, "target")
	rel, err := resolveArchive(target)
	if err != nil {
		return file.Failure(toolAdd, err)
	}
	if _, err := filesupport.BackupWithOp(rel, "archive_add"); err != nil {
		return file.Failure(toolAdd, fmt.Errorf("保存版本失败：%w", err))
	}
	entry := file.StringArg(args, "entry")
	source := file.StringArg(args, "source")
	if source != "" {
		resolved, _, resolveErr := filesupport.Resolve(source, c.root)
		if resolveErr != nil {
			return file.Failure(toolAdd, resolveErr)
		}
		source = resolved
	}
	result := ArchiveOps.AddEntry(target, entry, source, file.StringArg(args, "content"))
	return mutated(c, rel, result, "向归档 "+rel+" 添加条目 "+entry)
}

func archiveExtract(args map[string]any, c *collector) file.Outcome {
	target := file.StringArg(args, "target")
	entryName := file.StringArg(args, "entry")
	if entryName == "" {
		return file.Failure(toolExtract, fmt.Errorf("entry 不能为空"))
	}
	_, archiveRel, err := ArchiveReader.ResolveArchivePath(target)
	if err != nil {
		return file.Failure(toolExtract, err)
	}
	outputDir := file.StringArg(args, "output_dir")
	if outputDir == "" {
		outputDir = path.Dir(archiveRel)
	}
	outputRel := path.Join(outputDir, path.Base(entryName))
	outputAbs := filesupport.Abs(outputRel)
	if _, err := filesupport.BackupWithOp(outputRel, "archive_extract"); err != nil {
		return file.Failure(toolExtract, fmt.Errorf("保存版本失败：%w", err))
	}
	reader, err := zip.OpenReader(filesupport.Abs(archiveRel))
	if err != nil {
		return file.Failure(toolExtract, err)
	}
	defer reader.Close()
	for _, member := range reader.File {
		if member.Name != entryName {
			continue
		}
		if member.FileInfo().IsDir() {
			return file.Failure(toolExtract, fmt.Errorf("目标条目是目录"))
		}
		if err := os.MkdirAll(filepath.Dir(outputAbs), 0o755); err != nil {
			return file.Failure(toolExtract, err)
		}
		src, openErr := member.Open()
		if openErr != nil {
			return file.Failure(toolExtract, openErr)
		}
		dst, createErr := os.Create(outputAbs)
		if createErr != nil {
			src.Close()
			return file.Failure(toolExtract, createErr)
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			return file.Failure(toolExtract, copyErr)
		}
		handle := ""
		if _, registered, regErr := resource.RegisterPath(outputRel); regErr == nil {
			handle = registered
		}
		outcome := file.Outcome{
			JSON:     file.JSON(map[string]any{"ok": true, "path": outputRel, "entry": entryName, "handle": handle}),
			Summary:  "已从 " + archiveRel + " 提取 " + entryName,
			Mutated:  true,
			Evidence: "提取 " + archiveRel + " 的 " + entryName + " 到 " + outputRel,
		}
		if handle != "" {
			outcome.Handles = []string{handle}
			c.addHandle(handle)
		}
		c.addChanged(outputRel)
		c.addEvidence(outcome.Evidence)
		return outcome
	}
	return file.Failure(toolExtract, fmt.Errorf("归档中找不到条目 %s", entryName))
}

func archiveList(args map[string]any) file.Outcome {
	target := file.StringArg(args, "target")
	entries, err := ArchiveReader.ListEntries(target)
	if err != nil {
		return file.Failure(toolList, err)
	}
	return file.Outcome{
		JSON:    file.JSON(map[string]any{"ok": true, "target": target, "entries": entries}),
		Summary: fmt.Sprintf("%s 共 %d 个条目", target, len(entries)),
	}
}

func resolveArchive(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("target 不能为空")
	}
	_, rel, err := ArchiveReader.ResolveArchivePath(target)
	return rel, err
}

func mutated(c *collector, rel string, result ArchiveOps.Result, evidence string) file.Outcome {
	outcome := file.Outcome{JSON: result.JSON(), Summary: result.Summary, Handles: result.Handles}
	if result.OK {
		outcome.Mutated = true
		c.addHandle(result.Handles...)
		c.addChanged(rel)
		c.addEvidence(evidence)
	}
	return outcome
}

func withHandlePaths(instruction string, handles []string) string {
	lines := make([]string, 0, len(handles))
	for _, handle := range handles {
		entry, err := resource.Resolve(handle)
		if err == nil && strings.TrimSpace(entry.BlobPath) != "" {
			lines = append(lines, "- "+handle+" → "+entry.BlobPath)
		} else {
			lines = append(lines, "- "+handle)
		}
	}
	if len(lines) == 0 {
		return instruction
	}
	return instruction + "\n\n【目标句柄】\n" + strings.Join(lines, "\n")
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
