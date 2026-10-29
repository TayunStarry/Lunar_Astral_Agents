package resource

// gc_run.go 启动 GC 的唯一编排入口。
//
// 每次进程启动（Init 成功后）执行一次：
//  1. 处理 GC 表 1：把仍在主表的孤儿句柄移除，空闲句柄行保留供后续复用；
//  2. 扫描全库，把「源丢失且无文本描述」的句柄补登进表 1（有描述的一律保留）；
//  3. 建立 / 校准 GC 表 2：只覆盖 multimedia 根图片视频、audios 根哈希音频；
//  4. 结算调用积分：上一轮未用 -1、用过 +1，硬限制 [-20, 20]，归零当前调用；
//  5. 积分达到 -20 时清退：无描述者删除句柄与全部关联物理文件；
//     有描述者只删物理本体与派生缓存，句柄保留并降级为 lost；
//  6. 最后执行原有配额回收（保留等级 + 最久未用），与新机制叠加而非替换。

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// RunStartupGC 执行一次完整的启动 GC（幂等）。
func RunStartupGC() error {
	if err := EnsureReady(); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()

	if archiveDB == nil || archiveDB.db == nil {
		return fmt.Errorf("句柄库未初始化")
	}
	orphans, removed := archiveDB.processOrphanFreeListLocked()
	archived := archiveDB.collectOrphansLocked()
	// 先结算上一轮已有档案，再登记本轮新增媒体：新档案初始积分固定为 0，
	// 不参与本次启动的「上一轮未使用 -1」结算。
	pruned := archiveDB.pruneGCMediaScoresLocked()
	scoreRemoved := archiveDB.applyGCScoringLocked()
	tracked := archiveDB.seedGCMediaScoresLocked()
	quotaRemoved := archiveDB.runQuotaGCLocked()

	logger.Info("Persistence",
		"启动 GC 完成：空闲句柄池=%d 清理孤儿=%d 新登记孤儿=%d 可清理媒体=%d 失效档案=%d 积分删除=%d 配额回收=%d",
		orphans, removed, archived, tracked, pruned, scoreRemoved, quotaRemoved)
	return nil
}

// processOrphanFreeListLocked 处理 GC 表 1 的既有记录：
// 主表仍存在的孤儿句柄移除；已恢复 / 已有描述的记录从池中剔除。
func (d *store) processOrphanFreeListLocked() (pool int, removed int) {
	rows, err := d.db.Query(`SELECT handle, kind, seq FROM gc_orphan_handle ORDER BY created_at ASC`)
	if err != nil {
		return 0, 0
	}
	type orphanRow struct {
		handle string
		kind   string
		seq    int64
	}
	list := []orphanRow{}
	for rows.Next() {
		var row orphanRow
		if err := rows.Scan(&row.handle, &row.kind, &row.seq); err == nil {
			list = append(list, row)
		}
	}
	rows.Close()

	for _, row := range list {
		res, found := d.GetResourceByHandle(row.kind, row.seq)
		if !found {
			// 主表已无该句柄：池行继续保留，后续分配优先复用
			pool++
			continue
		}
		if !isOrphanCandidate(res) {
			// 源已恢复或已有文本描述：不再属于表 1
			_, _ = d.db.Exec(`DELETE FROM gc_orphan_handle WHERE handle = ?`, row.handle)
			continue
		}
		if err := d.addOrphanLocked(res, gcOrphanReason(res)); err == nil {
			pool++
			removed++
		}
	}
	return pool, removed
}

// collectOrphansLocked 扫描全库补登孤儿句柄（有文本描述永远不在此列）。
func (d *store) collectOrphansLocked() int {
	resources, err := d.ListAllResources()
	if err != nil {
		return 0
	}
	count := 0
	for _, res := range resources {
		if !isOrphanCandidate(res) {
			continue
		}
		if err := d.addOrphanLocked(res, gcOrphanReason(res)); err == nil {
			count++
		}
	}
	return count
}

// seedGCMediaScoresLocked 为可清理根目录媒体建档（初始积分 0、当前调用 0）。
func (d *store) seedGCMediaScoresLocked() int {
	resources, err := d.ListAllResources()
	if err != nil {
		return 0
	}
	count := 0
	for _, res := range resources {
		if !gcTrackablePath(res.BlobPath, res.Kind) || sourceMissing(res) {
			continue
		}
		before := gcMediaScoreExists(d, res.Handle)
		d.upsertGCMediaScoreLocked(&res)
		if !before {
			count++
		}
	}
	return count
}

// gcMediaScoreExists 判断 GC 表 2 是否已有该句柄（仅用于统计新登记数）。
func gcMediaScoreExists(d *store, handle string) bool {
	var count int
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM gc_media_score WHERE handle = ?`, handle).Scan(&count)
	return count > 0
}

// pruneGCMediaScoresLocked 移除已失效的档案行：
// 句柄已不在主表、路径不再属于可清理范围、或源文件已丢失。
func (d *store) pruneGCMediaScoresLocked() int {
	rows, err := d.db.Query(`SELECT handle, kind, blob_path FROM gc_media_score`)
	if err != nil {
		return 0
	}
	type scoreRef struct {
		handle   string
		kind     string
		blobPath string
	}
	refs := []scoreRef{}
	for rows.Next() {
		var ref scoreRef
		if err := rows.Scan(&ref.handle, &ref.kind, &ref.blobPath); err == nil {
			refs = append(refs, ref)
		}
	}
	rows.Close()

	// 必须先关闭结果集再点查：句柄库只保留 1 个连接，查询中再开查询会自锁。
	stale := []string{}
	for _, ref := range refs {
		res, found := d.GetResourceByHandle(ref.kind, handleSeq(ref.kind, ref.handle))
		if !found || !gcTrackablePath(ref.blobPath, ref.kind) || sourceMissing(res) {
			stale = append(stale, ref.handle)
		}
	}
	for _, handle := range stale {
		_, _ = d.db.Exec(`DELETE FROM gc_media_score WHERE handle = ?`, handle)
	}
	return len(stale)
}

// handleSeq 从句柄文本解析序号（非法句柄返回 0）。
func handleSeq(kind, handle string) int64 {
	parsed, ok := ParseHandle(handle)
	if !ok || parsed.Kind != kind {
		return 0
	}
	return parsed.Seq
}

// applyGCScoringLocked 结算上一轮调用积分，并在 -20 时清退句柄与物理文件。
func (d *store) applyGCScoringLocked() int {
	rows, err := d.db.Query(`SELECT handle, kind, blob_path, call_points, current_call FROM gc_media_score`)
	if err != nil {
		return 0
	}
	type scoreRow struct {
		handle      string
		kind        string
		blobPath    string
		callPoints  int
		currentCall int
	}
	list := []scoreRow{}
	for rows.Next() {
		var row scoreRow
		if err := rows.Scan(&row.handle, &row.kind, &row.blobPath, &row.callPoints, &row.currentCall); err == nil {
			list = append(list, row)
		}
	}
	rows.Close()

	removed := 0
	for _, row := range list {
		points := row.callPoints
		if row.currentCall > 0 {
			points += GCPointsUsedStep
		} else {
			points += GCPointsUnusedStep
		}
		if points > GCPointsMax {
			points = GCPointsMax
		}
		if points < GCPointsMin {
			points = GCPointsMin
		}
		if points <= GCPointsMin {
			if err := d.purgeScoredResourceLocked(row.handle, row.kind, row.blobPath); err == nil {
				removed++
			}
			continue
		}
		_, _ = d.db.Exec(`UPDATE gc_media_score SET call_points = ?, current_call = 0, updated_at = ?
			WHERE handle = ?`, points, time.Now().Unix(), row.handle)
	}
	return removed
}

// purgeScoredResourceLocked 积分达到 -20 时的清退：
//   - 无文本描述：删除句柄记录 + GC 表记录 + 全部关联物理文件；
//   - 有文本描述：只删除物理本体与派生缓存，保留句柄并降级为 lost。
func (d *store) purgeScoredResourceLocked(handle, kind, blobPath string) error {
	if res, found := d.GetResourceByHandle(kind, handleSeq(kind, handle)); found {
		d.purgeResourceArtifactsLocked(res)
		if hasTextDescription(res) {
			// 描述仍在：句柄必须保留，仅把本体标记为已回收，并停止继续累计积分。
			_, _ = d.db.Exec(`DELETE FROM cache WHERE source_url = ?`, res.URL)
			_, err := d.db.Exec(`UPDATE url_master SET state = ? WHERE id = ?`, StateLost, res.ID)
			d.removeGCScoreLocked(handle)
			return err
		}
		if err := d.deleteResourceLocked(res.ID); err != nil {
			return err
		}
	} else {
		d.purgeArtifactPathLocked(blobPath, kind)
	}
	_, _ = d.db.Exec(`DELETE FROM gc_media_score WHERE handle = ?`, handle)
	_, _ = d.db.Exec(`DELETE FROM gc_orphan_handle WHERE handle = ?`, handle)
	return nil
}

// purgeResourceArtifactsLocked 删除资源本体与派生缓存文件（只限 GC 可清理范围）。
// 调用方须持有 archiveMu。
func (d *store) purgeResourceArtifactsLocked(res Resource) {
	d.purgeArtifactPathLocked(res.BlobPath, res.Kind)
	cacheRows, err := d.db.Query(`SELECT path FROM cache WHERE source_url = ?`, res.URL)
	if err != nil {
		return
	}
	paths := []string{}
	for cacheRows.Next() {
		var path string
		if cacheRows.Scan(&path) == nil && path != "" {
			paths = append(paths, path)
		}
	}
	cacheRows.Close()
	for _, path := range paths {
		abs := localAbs(path)
		if base := filepath.Dir(abs); base != "" {
			_ = os.Remove(abs)
		}
	}
}

// purgeArtifactPathLocked 仅删除落在 GC 可清理范围内的物理文件，防止误删文档。
func (d *store) purgeArtifactPathLocked(blobPath, kind string) {
	if !gcTrackablePath(blobPath, kind) {
		return
	}
	_ = os.Remove(localAbs(blobPath))
}

// quotaBytes 内容仓库配额（字节）
func quotaBytes() int64 {
	if config.ArchiveQuotaMB <= 0 {
		return 0
	}
	return int64(config.ArchiveQuotaMB) * 1024 * 1024
}
