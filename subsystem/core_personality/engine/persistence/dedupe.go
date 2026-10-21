package persistence

// dedupe.go 内容哈希唯一性维护
//
// 句柄体系要求「同一内容哈希只对应一份实体」：
//   - 生成目录 / 集合子目录：优先级最高
//   - audios、multimedia 根目录：优先级次之（重复时优先舍弃）
//   - 派生缓存（multimedia/vision、multimedia/footage）：不入索引，只清历史目录行，
//     绝不删除副本文件——副本由引擎缓存表引用，可重建但删除会影响正在进行的读取
//
// 启动建表后、目录扫描后都会执行一次收敛，删除重复文件与重复目录行，
// 保留优先级最高的那一份；同优先级时保留最早分配的句柄（编号稳定）。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"os"
	"path/filepath"
)

// DedupeDuplicateHashes 收敛全部重复内容哈希，返回删除的重复行数
func (d *ArchiveDB) DedupeDuplicateHashes() (int, error) {
	rows, err := d.db.Query(`SELECT ` + archiveResourceColumns +
		` FROM resources WHERE content_hash <> '' ORDER BY kind, content_hash, id`)
	if err != nil {
		return 0, err
	}
	groups := map[string][]ArchiveResource{}
	for rows.Next() {
		res, scanErr := scanResource(rows)
		if scanErr != nil {
			continue
		}
		key := res.Kind + "\x00" + res.ContentHash
		groups[key] = append(groups[key], res)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	removed := 0
	for _, list := range groups {
		if len(list) < 2 {
			continue
		}
		// 选出保留项：优先 active 且实体在位者；其次按路径优先级；同条件保留最早分配的句柄
		canonical := list[0]
		for _, res := range list[1:] {
			if dedupePriority(res) > dedupePriority(canonical) {
				canonical = res
			}
		}
		for _, res := range list {
			if res.ID == canonical.ID {
				continue
			}
			// 派生缓存不在句柄体系内：只清历史目录行，副本文件留给缓存表按 LRU 管理。
			if IsDerivedStoragePath(res.BlobPath) {
				if err := d.DeleteResource(res.ID); err == nil {
					removed++
				}
				continue
			}
			// 同一实体路径只删目录行；不同路径且位于内容仓库树内才删除重复文件。
			// 仓库外的路径（历史工作区引用等）只删目录行，绝不删除用户原文件。
			if res.BlobPath != "" && res.BlobPath != canonical.BlobPath && IsStoragePath(res.BlobPath) {
				abs := filepath.Join(localDir, filepath.FromSlash(res.BlobPath))
				if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
					LoggerGeneral.SubWarn("Persistence", "Dedupe", "删除重复实体失败(%s): %v", res.BlobPath, err)
				}
			}
			if err := d.DeleteResource(res.ID); err != nil {
				LoggerGeneral.SubWarn("Persistence", "Dedupe", "删除重复句柄失败(%s): %v", res.Handle, err)
				continue
			}
			removed++
		}
		LoggerGeneral.Info("Persistence", "内容哈希去重: %s/%s 保留 %s(%s)，删除 %d 份重复实体",
			canonical.Kind, canonical.ContentHash, canonical.Handle, canonical.BlobPath, len(list)-1)
	}
	return removed, nil
}

// dedupePriority 去重时的保留优先级（越大越优先）
//
// 失效句柄或实体已丢失的行优先级最低；其余按路径优先级排序。
func dedupePriority(res ArchiveResource) int {
	if !IsStoragePath(res.BlobPath) {
		return -1
	}
	abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))
	if res.State != StateActive || !fileExists(abs) {
		return -1
	}
	return pathPriority(res.BlobPath)
}
