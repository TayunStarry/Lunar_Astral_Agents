package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BuildUpdatePackage 生成更新包分卷、清单与校验值（不涉及网络）。
// outDir 为空时使用程序根下的 .update/pack 临时目录。
func BuildUpdatePackage(root string, cfg *ReleaseConfig, version, outDir string, report func(Progress)) (*PackResult, error) {
	if report == nil {
		report = func(Progress) {}
	}
	if version == "" {
		return nil, fmt.Errorf("版本号不能为空")
	}
	if strings.ContainsAny(version, `/\:`) || strings.Contains(version, "..") {
		return nil, fmt.Errorf("版本号包含非法字符: %s", version)
	}
	sevenZip, err := findSevenZipFor(root, cfg.SevenZip)
	if err != nil {
		return nil, err
	}
	if outDir == "" {
		outDir = filepath.Join(updateRoot(root), "pack")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	files, err := collectPackFiles(root, cfg, report)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, f := range files {
		total += f.Size
	}
	manifest := &Manifest{
		Schema:  updateSchema,
		Version: version,
		BuiltAt: time.Now().Format(time.RFC3339),
		Files:   files,
	}
	manifestPath := filepath.Join(root, manifestFileName)
	restore, err := stashManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	defer restore()
	if err := writeJSONAtomic(manifestPath, manifest); err != nil {
		return nil, fmt.Errorf("写入更新包清单失败: %w", err)
	}

	listPath := filepath.Join(outDir, "files.txt")
	list := make([]string, 0, len(files)+1)
	list = append(list, manifestFileName)
	for _, f := range files {
		list = append(list, filepath.FromSlash(f.Path))
	}
	if err := os.WriteFile(listPath, []byte(strings.Join(list, "\n")), 0o644); err != nil {
		return nil, err
	}
	defer os.Remove(listPath)

	outBase := filepath.Join(outDir, updateAssetPrefix+version)
	if matches, _ := filepath.Glob(outBase + ".7z*"); len(matches) > 0 {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}

	report(Progress{Phase: "pack", Done: 0, Total: 1, Message: fmt.Sprintf("压缩 %d 个文件（%s）", len(files), humanBytes(total))})
	args := []string{
		"a", "-t7z",
		fmt.Sprintf("-v%dm", packSizeMB(cfg)),
		fmt.Sprintf("-mx=%d", packLevel(cfg)),
		"-mmt",
		"-bso0", "-bsp0",
		outBase + ".7z",
		"@" + listPath,
	}
	cmd := exec.Command(sevenZip, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("7z 分卷打包失败: %v: %s", err, truncateText(string(out), 800))
	}

	volumes, err := filepath.Glob(outBase + ".7z*")
	if err != nil || len(volumes) == 0 {
		return nil, fmt.Errorf("未找到打包产物")
	}
	sort.Slice(volumes, func(i, j int) bool {
		return volumeIndex(filepath.Base(volumes[i])) < volumeIndex(filepath.Base(volumes[j]))
	})

	shas := make(map[string]string, len(volumes))
	for i, vol := range volumes {
		sum, err := hashFileSHA256(vol)
		if err != nil {
			return nil, err
		}
		shas[filepath.Base(vol)] = sum
		report(Progress{Phase: "hash_volume", Done: i + 1, Total: len(volumes), Percent: percent(i+1, len(volumes)), Message: filepath.Base(vol)})
	}

	return &PackResult{
		Version:   version,
		Volumes:   volumes,
		SHAs:      shas,
		Manifest:  manifest,
		FileCount: len(files),
		Bytes:     total,
	}, nil
}

// stashManifest 暂存程序根下可能存在的同名清单文件，返回恢复函数。
func stashManifest(path string) (func(), error) {
	if !fileExists(path) {
		return func() { _ = os.Remove(path) }, nil
	}
	backup := path + ".bak"
	if err := os.Rename(path, backup); err != nil {
		return nil, fmt.Errorf("暂存同名清单文件失败: %w", err)
	}
	return func() {
		_ = os.Remove(path)
		_ = os.Rename(backup, path)
	}, nil
}

// collectPackFiles 枚举打包范围并计算每个文件的大小与 SHA-256。
func collectPackFiles(root string, cfg *ReleaseConfig, report func(Progress)) ([]FileEntry, error) {
	paths, err := enumeratePackPaths(root, cfg)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("打包范围为空，请检查 include 与 program_files")
	}
	files := make([]FileEntry, 0, len(paths))
	var total int64
	for i, rel := range paths {
		abs, err := safeJoin(root, rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("打包文件缺失: %s", rel)
		}
		sum, err := hashFileSHA256(abs)
		if err != nil {
			return nil, err
		}
		files = append(files, FileEntry{Path: rel, Size: info.Size(), SHA256: sum})
		total += info.Size()
		report(Progress{Phase: "manifest", Done: i + 1, Total: len(paths), Percent: percent(i+1, len(paths)), Message: rel})
	}
	return files, nil
}

// enumeratePackPaths 返回相对程序根的文件路径（去重、排序）。
func enumeratePackPaths(root string, cfg *ReleaseConfig) ([]string, error) {
	seen := make(map[string]bool)
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if rel != "" && !seen[rel] {
			seen[rel] = true
		}
	}

	for _, rel := range cfg.ProgramFiles {
		abs, err := safeJoin(root, rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("主程序文件不存在: %s", rel)
		}
		add(rel)
	}

	for _, inc := range cfg.Include {
		abs, err := safeJoin(root, inc)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("打包目录不存在: %s", inc)
		}
		if !info.IsDir() {
			if !hardExcluded(root, abs) {
				add(inc)
			}
			continue
		}
		if err := walkPackDir(root, abs, cfg, add); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(seen))
	for rel := range seen {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// walkPackDir 递归枚举目录，应用排除规则与目录级标记。
func walkPackDir(root, dir string, cfg *ReleaseConfig, add func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	patterns := excludePatterns(cfg)
	markers := excludeDirMarkers(cfg)
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if hardExcluded(root, full) {
			continue
		}
		if e.IsDir() {
			if nameExcluded(patterns, e.Name(), true) || dirHasAnyMarker(full, markers) {
				continue
			}
			if err := walkPackDir(root, full, cfg, add); err != nil {
				return err
			}
			continue
		}
		if nameExcluded(patterns, e.Name(), false) {
			continue
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		add(rel)
	}
	return nil
}

// hardExcluded 更新器自身工作区、日志、密钥与生成文件永不进包。
func hardExcluded(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return true
	}
	if rel == updateDirName || strings.HasPrefix(rel, updateDirName+"/") {
		return true
	}
	if rel == ".backup" || strings.HasPrefix(rel, ".backup/") {
		return true
	}
	if strings.HasPrefix(rel, "local_data/logs/") || rel == "local_data/logs" {
		return true
	}
	switch rel {
	case manifestFileName,
		"local_data/config/release.json",
		"local_data/config/version.json":
		return true
	}
	return false
}

// dirHasAnyMarker 判断目录内是否含目录级排除标记文件。
func dirHasAnyMarker(dir string, markers []string) bool {
	for _, name := range markers {
		if name == "" {
			continue
		}
		if fileExists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// nameExcluded 判断名称是否命中排除规则。
func nameExcluded(patterns []string, name string, isDir bool) bool {
	for _, p := range patterns {
		dirPattern := strings.HasSuffix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		if dirPattern != isDir {
			if dirPattern {
				continue
			}
		}
		if matchPackPattern(p, name) {
			return true
		}
	}
	return false
}

// matchPackPattern 支持 *.ext、精确名与已知目录名。
func matchPackPattern(pattern, name string) bool {
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(name, strings.TrimPrefix(pattern, "*"))
	}
	switch pattern {
	case "node_modules", "__pycache__", "dist", "build", ".git":
		return name == pattern
	}
	return name == pattern
}
