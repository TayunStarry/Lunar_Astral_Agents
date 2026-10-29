package Creator

// dts.go DTS 开发文档查阅：只读 LTP9 引擎自带文档目录。
// 文档目录随源码/发行版放置，按可执行目录与工作目录的候选路径定位；
// 读取限定在文档目录内，并只允许 .d.ts / .md / .txt 文件。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dtsCandidateRoots 文档目录候选路径。
func dtsCandidateRoots() []string {
	roots := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(exeDir, "engine", "plugin", "docs"),
			filepath.Join(exeDir, "subsystem", "lunar_harness", "engine", "plugin", "docs"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots,
			filepath.Join(cwd, "engine", "plugin", "docs"),
			filepath.Join(cwd, "subsystem", "lunar_harness", "engine", "plugin", "docs"),
		)
	}
	return roots
}

// findDTSRoot 返回第一个存在的文档目录。
func findDTSRoot() (string, bool) {
	for _, root := range dtsCandidateRoots() {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root, true
		}
	}
	return "", false
}

// dtsAllowed 只允许查阅类型声明与开发指南类文档。
func dtsAllowed(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".d.ts") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt")
}

// listDTSDocs 列出可查阅的文档。
func listDTSDocs() ([]docInfo, error) {
	root, ok := findDTSRoot()
	if !ok {
		return nil, fmt.Errorf("未找到 DTS 开发文档目录")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取文档目录失败: %w", err)
	}
	out := make([]docInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !dtsAllowed(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		out = append(out, docInfo{Name: entry.Name(), Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// readDTSDoc 读取指定文档；name 为空时返回文档清单文本。
func readDTSDoc(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		docs, err := listDTSDocs()
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(docs))
		for _, doc := range docs {
			names = append(names, fmt.Sprintf("%s（%d 字节）", doc.Name, doc.Size))
		}
		return "可用 DTS 文档：\n- " + strings.Join(names, "\n- "), nil
	}
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", fmt.Errorf("文档名必须是文档目录内的文件名")
	}
	if !dtsAllowed(name) {
		return "", fmt.Errorf("只允许查阅 .d.ts / .md / .txt 文档")
	}
	root, ok := findDTSRoot()
	if !ok {
		return "", fmt.Errorf("未找到 DTS 开发文档目录")
	}
	full := filepath.Join(root, filepath.Base(name))
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("文档不存在: %s", name)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s 是目录", name)
	}
	if info.Size() > maxDTSReadBytes {
		return "", fmt.Errorf("文档过大（%d 字节），上限 %d 字节", info.Size(), maxDTSReadBytes)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("读取文档失败: %w", err)
	}
	return previewRunes(string(raw), dtsMaxResultRunes), nil
}
