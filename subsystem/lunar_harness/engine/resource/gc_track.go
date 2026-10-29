package resource

// gc_track.go GC 专用表 1 / 表 2 的读写原语与句柄空闲池。
//
// 表 1（gc_orphan_handle）：源文件不存在且没有任何文本描述的句柄。启动时从主表
// 移除，但行本身保留为「空闲句柄池」，新资源优先复用这些句柄序号。
//
// 表 2（gc_media_score）：可清理的根目录媒体档案（multimedia 根图片/视频、
// audios 根哈希音频），记录调用积分与本轮是否被查询。

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// takeFreeOrNextSeqLocked 优先从 GC 表 1 的空闲池取句柄，池空时分配新序号。
// 调用方须持有 archiveMu。
func (d *store) takeFreeOrNextSeqLocked(kind string) (seq int64, handle string, err error) {
	if freeSeq, freeHandle, ok := d.takeFreeHandleLocked(kind); ok {
		return freeSeq, freeHandle, nil
	}
	seq, err = d.nextSeqLocked(kind)
	if err != nil {
		return 0, "", err
	}
	return seq, FormatHandle(kind, seq), nil
}

// takeFreeHandleLocked 从 GC 表 1 取一个空闲句柄；池空或记录失效时返回 false。
// 调用方须持有 archiveMu。
func (d *store) takeFreeHandleLocked(kind string) (seq int64, handle string, ok bool) {
	table := handleTable(kind)
	if table == "" {
		return 0, "", false
	}
	for {
		row := d.db.QueryRow(`SELECT handle, seq FROM gc_orphan_handle
			WHERE kind = ? ORDER BY created_at ASC, seq ASC LIMIT 1`, kind)
		var candidateHandle string
		var candidateSeq int64
		if err := row.Scan(&candidateHandle, &candidateSeq); err != nil {
			return 0, "", false
		}
		// 先摘除池行，避免并发分配同一句柄
		if _, err := d.db.Exec(`DELETE FROM gc_orphan_handle WHERE handle = ?`, candidateHandle); err != nil {
			return 0, "", false
		}
		// 失效记录（序号或句柄仍被主表占用）直接丢弃，继续取下一个
		var occupied int
		_ = d.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE seq = ? OR handle = ?`,
			candidateSeq, candidateHandle).Scan(&occupied)
		if occupied == 0 {
			return candidateSeq, candidateHandle, true
		}
	}
}

// rollbackSeqLocked 分配失败时回收序号缓存（仅回退「刚刚分配的最新序号」）。
func (d *store) rollbackSeqLocked(kind string, seq int64) {
	if archiveSeqCache[kind] == seq {
		archiveSeqCache[kind] = seq - 1
	}
}

// addOrphanLocked 把「源丢失且无文本描述」的资源移出句柄体系，并把句柄留在空闲池。
// 调用方须持有 archiveMu。
func (d *store) addOrphanLocked(res Resource, reason string) error {
	if res.Handle == "" || hasTextDescription(res) {
		return nil
	}
	now := time.Now().Unix()
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO gc_orphan_handle
		(handle, kind, seq, url, blob_path, reason, created_at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(handle) DO UPDATE SET kind = excluded.kind, seq = excluded.seq,
		url = excluded.url, blob_path = excluded.blob_path, reason = excluded.reason`,
		res.Handle, res.Kind, res.Seq, res.URL, res.BlobPath, reason, now); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM `+handleTable(res.Kind)+` WHERE seq = ?`, res.Seq); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM doc_sections WHERE url = ?`, res.URL); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cache WHERE source_url = ?`, res.URL); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM gc_media_score WHERE handle = ?`, res.Handle); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM url_master WHERE id = ?`, res.ID); err != nil {
		rollbackTx(tx)
		return err
	}
	return tx.Commit()
}

// ArchiveOrphan 对一个已定位的资源执行孤儿归档（加锁入口）。
func (d *store) ArchiveOrphan(res Resource, reason string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	return d.addOrphanLocked(res, reason)
}

// preserveOrArchive 源不可用时统一处置：
// 有文本描述 → 只标记 lost；无描述 → 移出主表并进入空闲池。
func (d *store) preserveOrArchive(res Resource, reason string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	if hasTextDescription(res) {
		_, err := d.db.Exec(`UPDATE url_master SET state = ? WHERE id = ?`, StateLost, res.ID)
		return err
	}
	return d.addOrphanLocked(res, reason)
}

// markGCCurrentCall 记录「本轮该句柄被文件查询调用过」，供下次启动结算积分。
func (d *store) markGCCurrentCall(handle string) {
	if handle == "" {
		return
	}
	_, _ = d.db.Exec(`UPDATE gc_media_score SET current_call = ?, updated_at = ?
		WHERE handle = ?`, gcCurrentCallUsed, time.Now().Unix(), handle)
}

// upsertGCMediaScoreLocked 在入库 / 扫描登记时确保可清理媒体进入 GC 表 2。
// 已有记录只刷新类型与路径，积分与当前调用保持不变。
func (d *store) upsertGCMediaScoreLocked(res *Resource) {
	if !gcTrackablePath(res.BlobPath, res.Kind) {
		return
	}
	now := time.Now().Unix()
	_, _ = d.db.Exec(`INSERT INTO gc_media_score
		(handle, kind, blob_path, call_points, current_call, created_at, updated_at)
		VALUES (?,?,?,?,0,?,?)
		ON CONFLICT(handle) DO UPDATE SET kind = excluded.kind, blob_path = excluded.blob_path,
		updated_at = excluded.updated_at`,
		res.Handle, res.Kind, res.BlobPath, GCPointsInit, now, now)
}

// removeGCScoreLocked 资源被显式删除时同步移除 GC 表 2 记录。
func (d *store) removeGCScoreLocked(handle string) {
	if handle == "" {
		return
	}
	_, _ = d.db.Exec(`DELETE FROM gc_media_score WHERE handle = ?`, handle)
}

// gcTrackablePath 是否属于 GC 表 2 的可清理范围：
//   - multimedia 根目录下的图片 / 动图 / 视频；
//   - audios 根目录下以内容哈希命名的音频。
func gcTrackablePath(rel, kind string) bool {
	rel = normalizeRel(rel)
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[1] == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(parts[1]))
	switch parts[0] {
	case StorageRootMultimedia:
		return kind == KindImage || kind == KindAnimated || kind == KindVideo
	case StorageRootAudios:
		if kind != KindAudio {
			return false
		}
		return isHashAudioName(parts[1], ext)
	}
	return false
}

// isHashAudioName 判断 audios 根目录音频是否为 sha16 哈希命名
func isHashAudioName(filename, ext string) bool {
	base := strings.TrimSuffix(filename, ext)
	if len(base) != gcHashAudioNameLen {
		return false
	}
	for i := 0; i < len(base); i++ {
		c := base[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// hasTextDescription 句柄是否已关联文本描述（解析文本 / 摘要）。
// 只要存在描述，该句柄永远不会因源文件丢失被移除。
func hasTextDescription(res Resource) bool {
	return strings.TrimSpace(res.ParseText) != ""
}

// sourceMissing 资源的本地源是否不存在；外部链接（无本地实体）不按源丢失处理。
func sourceMissing(res Resource) bool {
	if strings.TrimSpace(res.BlobPath) == "" {
		return false
	}
	abs := localAbs(res.BlobPath)
	return !fileExists(abs)
}

// isOrphanCandidate 是否满足 GC 表 1 的全部条件。
func isOrphanCandidate(res Resource) bool {
	return res.Handle != "" && sourceMissing(res) && !hasTextDescription(res)
}

// gcOrphanReason 孤儿判定原因（写入表 1，便于运维排查）。
func gcOrphanReason(res Resource) string {
	if strings.TrimSpace(res.BlobPath) == "" {
		return "源地址不可用"
	}
	return fmt.Sprintf("源文件不存在: %s", res.BlobPath)
}
