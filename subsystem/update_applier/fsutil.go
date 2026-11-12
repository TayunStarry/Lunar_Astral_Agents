package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// writeFileAtomic 在同一目录写入临时文件并 rename，保证断电后不出现半截文件。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// writeJSONAtomic 以缩进 JSON 原子写入。
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o644)
}

// readJSONFile 读取 JSON；文件不存在时返回 os.ErrNotExist。
func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return nil
}

// safeJoin 把清单里的相对路径安全地拼到 root 下，拒绝绝对路径、盘符与 ..
func safeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("清单路径为空")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("清单路径不允许绝对路径: %s", rel)
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("清单路径不允许根相对路径: %s", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("清单路径越界: %s", rel)
	}
	joined := filepath.Join(root, clean)
	back, err := filepath.Rel(root, joined)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("清单路径越界: %s", rel)
	}
	return joined, nil
}

// fileSize 取文件大小，失败返回 0。
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// hashFileSHA256 流式计算文件 SHA-256（小写十六进制）。
func hashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ensureParentDir 创建文件所在目录，并登记到 created 列表（若目录原本不存在）。
func ensureParentDir(path string, created *[]string) error {
	dir := filepath.Dir(path)
	missing, err := collectMissingDirs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	*created = append(*created, missing...)
	return nil
}

// collectMissingDirs 自底向上收集不存在的目录（保持由浅到深）。
func collectMissingDirs(dir string) ([]string, error) {
	var reversed []string
	cur := dir
	for {
		info, err := os.Stat(cur)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("路径不是目录: %s", cur)
			}
			break
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		reversed = append(reversed, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	out := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		out = append(out, reversed[i])
	}
	return out, nil
}
