package resource

// gc_quota.go 原有配额回收逻辑（保留等级 + 最久未用）与表驱动 GC 的叠加层。
//
// 这里刻意只回收「归档私有副本」（blob_owned = 1），并且：
//   - 有文本描述的句柄只降级为 lost，不删除句柄；
//   - 无文本描述的句柄按 GC 表 1 规则移出主表并进入空闲池。

import "os"

// runQuotaGCLocked 在启动 GC 末尾执行配额回收，返回被回收的资源数。
func (d *store) runQuotaGCLocked() int {
	quota := quotaBytes()
	if quota <= 0 {
		return 0
	}
	total, err := d.totalActiveBytes()
	if err != nil || total <= quota {
		return 0
	}

	removed := 0
	for _, retain := range []string{RetainEphemeral, RetainSoft, RetainNormal} {
		if total <= quota {
			break
		}
		candidates, err := d.collectCandidates(retain, 500)
		if err != nil {
			continue
		}
		for _, res := range candidates {
			if total <= quota {
				break
			}
			if !res.BlobOwned || res.BlobPath == "" {
				continue
			}
			if !IsStoragePath(res.BlobPath) || IsDerivedStoragePath(res.BlobPath) {
				continue
			}
			abs := localAbs(res.BlobPath)
			if _, statErr := os.Stat(abs); statErr != nil {
				continue
			}
			if err := os.Remove(abs); err != nil {
				continue
			}
			total -= res.SizeBytes
			removed++
			if hasTextDescription(res) {
				// 有描述：句柄保留，只标记本体已回收
				_, _ = d.db.Exec(`UPDATE url_master SET state = ? WHERE id = ?`, StateLost, res.ID)
				continue
			}
			// 无描述：按孤儿句柄规则移出主表，句柄进入空闲池
			_ = d.addOrphanLocked(res, "配额回收")
		}
	}
	return removed
}
