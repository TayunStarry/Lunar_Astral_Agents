package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findSevenZipFor 在程序根、配置路径与系统安装路径中查找 7z。
func findSevenZipFor(root, configured string) (string, error) {
	candidates := []string{}
	if configured != "" {
		if filepath.IsAbs(configured) {
			candidates = append(candidates, configured)
		} else {
			candidates = append(candidates, filepath.Join(root, configured))
		}
	}
	candidates = append(candidates,
		filepath.Join(root, "local_data", "package", "archive", "7z.exe"),
		`C:\Program Files\7-Zip\7z.exe`,
		`C:\Program Files (x86)\7-Zip\7z.exe`,
	)
	if p, err := exec.LookPath("7z"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 7z 工具，请安装 7-Zip 或确认 local_data/package/archive/7z.exe 存在")
}

// testArchive 用 7z t 校验分卷完整性与首卷可读性。
func testArchive(sevenZip, firstVolume string) error {
	out, err := exec.Command(sevenZip, "t", firstVolume).CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z 完整性测试失败: %v: %s", err, truncateText(string(out), 800))
	}
	return nil
}

// extractVolumes 解压分卷到目标目录。
func extractVolumes(sevenZip, firstVolume, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	out, err := exec.Command(sevenZip, "x", "-y", "-o"+dest, firstVolume).CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z 解压失败: %v: %s", err, truncateText(string(out), 800))
	}
	return nil
}

// verifyExtracted 校验解压目录内的清单：版本一致、文件齐全、大小与 SHA-256 相符。
// 返回清单中可安装的文件列表。
func verifyExtracted(staging, expectedVersion string, report func(Progress)) ([]FileEntry, error) {
	if report == nil {
		report = func(Progress) {}
	}
	var m Manifest
	manifestPath := filepath.Join(staging, manifestFileName)
	if err := readJSONFile(manifestPath, &m); err != nil {
		return nil, fmt.Errorf("读取更新包清单失败: %w", err)
	}
	if m.Schema != updateSchema {
		return nil, fmt.Errorf("不支持的清单版本: %d", m.Schema)
	}
	if m.Version != expectedVersion {
		return nil, fmt.Errorf("清单版本 %s 与目标版本 %s 不一致", m.Version, expectedVersion)
	}
	if len(m.Files) == 0 {
		return nil, fmt.Errorf("更新包清单为空")
	}

	files := make([]FileEntry, 0, len(m.Files))
	for i, f := range m.Files {
		path, err := safeJoin(staging, f.Path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("清单文件缺失: %s", f.Path)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("清单项是目录: %s", f.Path)
		}
		if f.Size > 0 && info.Size() != f.Size {
			return nil, fmt.Errorf("文件大小不符: %s（期望 %d，实际 %d）", f.Path, f.Size, info.Size())
		}
		sum, err := hashFileSHA256(path)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(sum, f.SHA256) {
			return nil, fmt.Errorf("文件校验失败: %s（期望 %s，实际 %s）", f.Path, f.SHA256, sum)
		}
		files = append(files, FileEntry{Path: f.Path, Size: info.Size(), SHA256: sum})
		report(Progress{Phase: "verify_manifest", Done: i + 1, Total: len(m.Files), Percent: percent(i+1, len(m.Files)), Message: f.Path})
	}
	return files, nil
}
