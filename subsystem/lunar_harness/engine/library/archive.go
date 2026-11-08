package library

// archive.go 资源库压缩包只读预览：只读取条目元数据，不解压落盘、不执行包内任何内容。
//
// 能力边界：zip / tar / tar.gz（.tgz）可逐目录浏览；单文件 .gz 只视为一条条目，
// 体积取 gzip 尾部的 ISIZE（超过 4 GiB 会按模 2^32 截断）。

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// ArchiveHandler GET /library/api/archive?path=&dir=&offset=0&limit=200 — 压缩包条目分页。
func ArchiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	dir, err := normalizeArchiveDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ArchiveResponse{OK: false, Path: normalizeRel(rel), Error: err.Error()})
		return
	}
	full, err := resolveArchivePath(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ArchiveResponse{OK: false, Path: normalizeRel(rel), Dir: dir, Error: err.Error()})
		return
	}
	format, ok := archiveFormat(rel)
	if !ok {
		writeJSON(w, http.StatusBadRequest, ArchiveResponse{OK: false, Path: normalizeRel(rel), Dir: dir, Error: "不支持的压缩包格式（仅 zip / tar / tar.gz / tgz / gz）"})
		return
	}
	offset := queryInt(r, "offset")
	if offset < 0 {
		offset = 0
	}
	limit := queryInt(r, "limit")
	if limit <= 0 {
		limit = defaultArchivePageSize
	}
	if limit > maxArchivePageSize {
		limit = maxArchivePageSize
	}
	all, truncated, err := listArchive(full, format, dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ArchiveResponse{OK: false, Path: normalizeRel(rel), Format: format, Dir: dir, Error: err.Error()})
		return
	}
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, ArchiveResponse{
		OK:        true,
		Path:      normalizeRel(rel),
		Format:    format,
		Dir:       dir,
		Entries:   all[offset:end],
		Total:     total,
		Offset:    offset,
		Limit:     limit,
		HasMore:   end < total,
		Truncated: truncated,
	})
}

// archiveFormat 按扩展名判定压缩包格式（复合扩展名优先）。
func archiveFormat(rel string) (string, bool) {
	lower := strings.ToLower(normalizeRel(rel))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz", true
	case strings.HasSuffix(lower, ".tar"):
		return "tar", true
	case strings.HasSuffix(lower, ".zip"):
		return "zip", true
	case strings.HasSuffix(lower, ".gz"):
		return "gz", true
	}
	return "", false
}

// normalizeArchiveDir 规整包内目录，拒绝越级路径。
func normalizeArchiveDir(dir string) (string, error) {
	dir = strings.ReplaceAll(strings.TrimSpace(dir), "\\", "/")
	dir = strings.Trim(dir, "/")
	if dir == "" || dir == "." {
		return "", nil
	}
	cleaned := path.Clean(dir)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("目录不合法")
	}
	if strings.ContainsRune(cleaned, 0) {
		return "", fmt.Errorf("目录不合法")
	}
	return cleaned, nil
}

// archiveCollector 汇总指定目录的直接子项（目录由条目路径合成）。
type archiveCollector struct {
	dir       string
	children  map[string]ArchiveEntry
	scanned   int
	truncated bool
}

func newArchiveCollector(dir string) *archiveCollector {
	return &archiveCollector{dir: dir, children: make(map[string]ArchiveEntry, 64)}
}

// add 登记一个包内条目；返回 false 表示已达到扫描上限。
func (c *archiveCollector) add(raw string, size int64, modified int64, isDir bool) bool {
	c.scanned++
	if c.scanned > maxArchiveEntries {
		c.truncated = true
		return false
	}
	name := normalizeArchiveName(raw)
	if name == "" {
		return true
	}
	prefix := ""
	if c.dir != "" {
		prefix = c.dir + "/"
	}
	if !strings.HasPrefix(name, prefix) {
		return true
	}
	rest := strings.TrimPrefix(name, prefix)
	if rest == "" {
		return true
	}
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		child := rest[:idx]
		if child == "" {
			return true
		}
		if _, exists := c.children[child]; !exists {
			c.children[child] = ArchiveEntry{
				Name: child,
				Path: joinArchivePath(c.dir, child),
				Type: "dir",
			}
		}
		return true
	}
	kind := "file"
	if isDir {
		kind = "dir"
	}
	c.children[rest] = ArchiveEntry{
		Name:     rest,
		Path:     joinArchivePath(c.dir, rest),
		Type:     kind,
		Size:     size,
		Modified: modified,
	}
	return true
}

// sorted 目录优先、再按名称排序。
func (c *archiveCollector) sorted() []ArchiveEntry {
	list := make([]ArchiveEntry, 0, len(c.children))
	for _, entry := range c.children {
		list = append(list, entry)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Type != list[j].Type {
			return list[i].Type == "dir"
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// listArchive 读取压缩包并返回指定目录的直接子项。
func listArchive(full, format, dir string) ([]ArchiveEntry, bool, error) {
	collector := newArchiveCollector(dir)
	switch format {
	case "zip":
		reader, err := zip.OpenReader(full)
		if err != nil {
			return nil, false, fmt.Errorf("读取 ZIP 失败: %w", err)
		}
		defer reader.Close()
		for _, file := range reader.File {
			isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
			if !collector.add(file.Name, int64(file.UncompressedSize64), archiveUnix(file.Modified), isDir) {
				break
			}
		}
	case "tar":
		file, err := os.Open(full)
		if err != nil {
			return nil, false, fmt.Errorf("读取 TAR 失败: %w", err)
		}
		defer file.Close()
		if err := walkTar(tar.NewReader(file), collector); err != nil {
			return nil, false, fmt.Errorf("读取 TAR 失败: %w", err)
		}
	case "tar.gz":
		file, err := os.Open(full)
		if err != nil {
			return nil, false, fmt.Errorf("读取 TAR.GZ 失败: %w", err)
		}
		defer file.Close()
		unzipped, err := gzip.NewReader(file)
		if err != nil {
			return nil, false, fmt.Errorf("读取 TAR.GZ 失败: %w", err)
		}
		defer unzipped.Close()
		if err := walkTar(tar.NewReader(unzipped), collector); err != nil {
			return nil, false, fmt.Errorf("读取 TAR.GZ 失败: %w", err)
		}
	case "gz":
		info, err := os.Stat(full)
		if err != nil {
			return nil, false, fmt.Errorf("读取 GZ 失败: %w", err)
		}
		name := strings.TrimSuffix(path.Base(filepathSlash(full)), ".gz")
		collector.add(name, gzipSize(full, info.Size()), info.ModTime().Unix(), false)
	default:
		return nil, false, fmt.Errorf("不支持的压缩包格式")
	}
	return collector.sorted(), collector.truncated, nil
}

// walkTar 顺序扫描 tar 流；链接与设备条目跳过。
func walkTar(reader *tar.Reader, collector *archiveCollector) error {
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		isDir := header.Typeflag == tar.TypeDir
		if !isDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if !collector.add(header.Name, header.Size, archiveUnix(header.ModTime), isDir) {
			return nil
		}
	}
}

// archiveUnix 把零值时间归一为 0，避免前端展示异常年份。
func archiveUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// gzipSize 读取 gzip 尾部 ISIZE（解压后体积的低 32 位）。
func gzipSize(full string, total int64) int64 {
	if total < 4 {
		return 0
	}
	file, err := os.Open(full)
	if err != nil {
		return 0
	}
	defer file.Close()
	var tail [4]byte
	if _, err := file.ReadAt(tail[:], total-4); err != nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint32(tail[:]))
}

// normalizeArchiveName 统一斜杠、去掉 ./ 前缀与尾部斜杠，拒绝越级路径。
func normalizeArchiveName(raw string) string {
	name := strings.ReplaceAll(raw, "\\", "/")
	name = strings.TrimLeft(name, "/")
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == "." {
		return ""
	}
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return ""
	}
	if strings.ContainsRune(cleaned, 0) {
		return ""
	}
	return cleaned
}

func joinArchivePath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// filepathSlash 把本地路径转成正斜杠形式，供 path.Base 使用。
func filepathSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
