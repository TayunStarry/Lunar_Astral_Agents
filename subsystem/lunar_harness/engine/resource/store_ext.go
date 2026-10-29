package resource

// store_ext.go 路径维度的维护原语：启动扫描、文件移动 / 删除、内容刷新
//
// 本地资源的身份就是 /file/read/<rel> 这条 URL；路径变化等同 URL 变化，
// 因此改写路径必须同步句柄表里的 url、url_master 的 url 与 blob_path。

import (
	"path/filepath"
	"strings"
)

// ListAllResources 列出全部已绑定句柄的资源（启动扫描清理使用）
func (d *store) ListAllResources() ([]Resource, error) {
	rows, err := d.db.Query(`SELECT ` + urlMasterColumns + ` FROM url_master WHERE bound_handle <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		res, scanErr := scanURLRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// UpdateResourceScan 按扫描结果刷新实体元数据（保留解析文本 / 标题 / 标签）
func (d *store) UpdateResourceScan(id int64, name, ext, mime, hash string, size int64) error {
	return d.RefreshResourceContent(id, name, ext, mime, hash, size)
}

// UpdateResourceBlobPath 改写单行的实体路径（文件移动），同步句柄表 URL
func (d *store) UpdateResourceBlobPath(id int64, blobPath string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	res, ok := d.GetResourceByID(id)
	if !ok {
		return nil
	}
	newURL := NormalizeURL(FileReadURL(blobPath))
	if newURL == "" || newURL == res.URL {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if res.Handle != "" {
		if _, err := tx.Exec(`UPDATE `+handleTable(res.Kind)+` SET url = ? WHERE seq = ?`, newURL, res.Seq); err != nil {
			rollbackTx(tx)
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE url_master SET url = ?, blob_path = ? WHERE id = ?`,
		newURL, normalizeRel(blobPath), id); err != nil {
		rollbackTx(tx)
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// GC 表 2 跟随路径实时迁移：进入 multimedia/audios 根 → 建档；
	// 移出根目录 → 退表，后续不再累计引用积分。
	moved := res
	moved.BlobPath = normalizeRel(blobPath)
	moved.URL = newURL
	d.syncGCMediaScoreAfterPathLocked(&moved)
	return nil
}

// DeleteResourcesByBlobPath 删除实体路径精确匹配的全部资源
func (d *store) DeleteResourcesByBlobPath(blobPath string) error {
	blobPath = normalizeRel(blobPath)
	if blobPath == "" {
		return nil
	}
	return d.deleteResourceByURL(FileReadURL(blobPath))
}

// DeleteResourcesByBlobPrefix 删除实体路径位于目录前缀之下的全部资源
func (d *store) DeleteResourcesByBlobPrefix(prefix string) error {
	prefix = strings.TrimSuffix(normalizeRel(prefix), "/")
	if prefix == "" {
		return nil
	}
	rows, err := d.db.Query(`SELECT id FROM url_master WHERE blob_path = ? OR blob_path LIKE ?`,
		prefix, prefix+"/%")
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if err := d.DeleteResource(id); err != nil {
			return err
		}
	}
	return nil
}

// RewriteBlobPrefix 把旧目录（或单文件）下的资源路径改写为新位置（移动 / 重命名）
//
// oldRel == newRel 时视为单文件；否则按目录前缀处理。返回被改写的行数。
func (d *store) RewriteBlobPrefix(oldRel, newRel string) (int, error) {
	oldRel = strings.TrimSuffix(normalizeRel(oldRel), "/")
	newRel = strings.TrimSuffix(normalizeRel(newRel), "/")
	if oldRel == "" || oldRel == newRel {
		return 0, nil
	}
	rows, err := d.db.Query(`SELECT id, blob_path FROM url_master WHERE blob_path = ? OR blob_path LIKE ?`,
		oldRel, oldRel+"/%")
	if err != nil {
		return 0, err
	}
	type pending struct {
		id     int64
		newOne string
	}
	changes := []pending{}
	for rows.Next() {
		var id int64
		var blob string
		if scanErr := rows.Scan(&id, &blob); scanErr != nil {
			continue
		}
		changes = append(changes, pending{id: id, newOne: newRel + strings.TrimPrefix(blob, oldRel)})
	}
	rows.Close()
	for _, change := range changes {
		if err := d.UpdateResourceBlobPath(change.id, change.newOne); err != nil {
			return 0, err
		}
	}
	return len(changes), nil
}

// cleanupDerivedRows 清除历史遗留的派生缓存目录行（只删索引行，不删文件）
func (d *store) cleanupDerivedRows() {
	rows, err := d.db.Query(`SELECT id, blob_path FROM url_master WHERE bound_handle <> ''`)
	if err != nil {
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var blob string
		if scanErr := rows.Scan(&id, &blob); scanErr == nil && IsDerivedStoragePath(blob) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_ = d.DeleteResource(id)
	}
}

// isRootMediaPath 判断路径是否直接位于 audios/ 或 multimedia/ 根目录
func isRootMediaPath(rel string) bool {
	rel = normalizeRel(rel)
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		return false
	}
	return parts[0] == StorageRootAudios || parts[0] == StorageRootMultimedia
}

// pathRelOf 取相对 LocalDir 的斜杠路径（仅用于日志）
func pathRelOf(raw string) string {
	return filepath.ToSlash(raw)
}
