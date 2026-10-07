package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// organize 执行入库：枚举 → 哈希 → 首见内容移入 source_data → 原位置与重复文件以硬链接映射
// dryRun 为 true 时只统计不落盘
func organize(root string, exts map[string]bool, dryRun bool) (*warehouseStats, error) {
	stats := &warehouseStats{}

	records, err := walkRecords(root, exts)
	if err != nil {
		return nil, err
	}
	stats.Scanned = int64(len(records))

	sourceDir := filepath.Join(root, SourceDirName)
	// hash → 仓库实体路径（首见文件建立索引）
	index := make(map[string]string)

	for _, rec := range records {
		hash, err := hashFile(rec.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] 哈希失败: %s (%v)\n", rec.Path, err)
			stats.Failed++
			continue
		}

		store := storePath(sourceDir, hash, rec.Ext)
		known, seen := index[hash]
		if !seen {
			// 首见内容：移入仓库，原位置留硬链接
			if dryRun {
				stats.Moved++
				index[hash] = store
				continue
			}
			if err := os.MkdirAll(sourceDir, 0755); err != nil {
				return stats, err
			}
			if err := os.Rename(rec.Path, store); err != nil {
				fmt.Fprintf(os.Stderr, "[FAIL] 入库失败: %s (%v)\n", rec.Path, err)
				stats.Failed++
				continue
			}
			if err := linkFile(store, rec.Path); err != nil {
				// 链接失败则回滚，把文件移回原位，避免内容"消失"
				if rbErr := os.Rename(store, rec.Path); rbErr != nil {
					fmt.Fprintf(os.Stderr, "[FAIL] 回滚失败，实体滞留仓库: %s -> %s\n", store, rec.Path)
				}
				fmt.Fprintf(os.Stderr, "[FAIL] 建链失败: %s (%v)\n", rec.Path, err)
				stats.Failed++
				continue
			}
			index[hash] = store
			stats.Moved++
			continue
		}

		// 重复内容：若已是同一物理文件（上次运行留下的硬链接）则跳过，保证幂等
		if sameFile(rec.Path, known) {
			stats.Skipped++
			continue
		}
		if dryRun {
			stats.Linked++
			stats.SavedBytes += rec.Size
			continue
		}
		if err := replaceWithHardlink(rec.Path, known); err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] 去重失败: %s (%v)\n", rec.Path, err)
			stats.Failed++
			continue
		}
		stats.Linked++
		stats.SavedBytes += rec.Size
	}

	return stats, nil
}

// main 入口：media_warehouse -dir <目标目录> [-ext .jpg,.png] [-dry-run]
func main() {
	dir := flag.String("dir", "", "目标目录（递归处理其下全部图片/视频）")
	extList := flag.String("ext", "", "覆盖默认扩展名过滤，逗号分隔，如 .jpg,.png,.mp4")
	dryRun := flag.Bool("dry-run", false, "仅统计并预览，不做任何改动")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "用法: media_warehouse -dir <目标目录> [-ext .jpg,.png] [-dry-run]")
		os.Exit(2)
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] 目录解析失败: %v\n", err)
		os.Exit(1)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "[FAIL] 目录不存在: %s\n", root)
		os.Exit(1)
	}

	exts := DefaultExts
	if *extList != "" {
		exts = map[string]bool{}
		for _, e := range strings.Split(*extList, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts[e] = true
		}
	}

	mode := "执行入库"
	if *dryRun {
		mode = "DRY-RUN（预览，不改动）"
	}
	fmt.Printf("目标: %s\n模式: %s\n----\n", root, mode)

	stats, err := organize(root, exts, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] %v\n", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(out))
}
