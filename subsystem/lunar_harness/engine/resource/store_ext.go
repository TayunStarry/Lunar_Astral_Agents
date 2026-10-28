package resource

// store_ext.go 资源目录的扩展查询与写入原语
//
// 供启动扫描、文件操作（写入 / 删除 / 移动）与记忆库图片落盘复用。
// 所有句柄的分配仍由 store.go 的 InsertResource 统一完成，本文件只做
// 「按路径 / 前缀定位与改写」这类维护性操作。

import (
	"path/filepath"
	"strings"
)

// ListAllResources 列出全部资源目录行（启动扫描清理使用，不受分页限制）
func (d *store) ListAllResources() ([]Resource, error) {
	rows, err := d.db.Query(`SELECT ` + archiveResourceColumns + ` FROM resources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// UpdateResourceScan 按扫描结果刷新一行与实体相关的元数据（保留标题 / 摘要 / 标签）
func (d *store) UpdateResourceScan(id int64, name, ext, mime, hash string, size int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET name = ?, ext = ?, mime = ?, content_hash = ?,
		size_bytes = ?, state = ? WHERE id = ?`,
		name, ext, mime, hash, size, StateActive, id)
	return err
}

// UpdateResourceBlobPath 改写单行的实体路径（文件移动）
func (d *store) UpdateResourceBlobPath(id int64, blobPath string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET blob_path = ? WHERE id = ?`, blobPath, id)
	return err
}

// DeleteResourcesByBlobPath 删除实体路径精确匹配的全部目录行（文件删除 / 损坏清理）
func (d *store) DeleteResourcesByBlobPath(blobPath string) error {
	if blobPath == "" {
		return nil
	}
	rows, err := d.db.Query(`SELECT id FROM resources WHERE blob_path = ?`, blobPath)
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

// DeleteResourcesByBlobPrefix 删除实体路径位于目录前缀之下的全部目录行（目录删除）
func (d *store) DeleteResourcesByBlobPrefix(prefix string) error {
	prefix = strings.TrimSuffix(filepath.ToSlash(prefix), "/")
	if prefix == "" {
		return nil
	}
	rows, err := d.db.Query(`SELECT id FROM resources WHERE blob_path = ? OR blob_path LIKE ?`,
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

// RewriteBlobPrefix 把旧目录（或单文件）下的全部实体路径改写为新位置（移动 / 重命名）
//
// prefix 语义：oldRel == newRel 时视为单文件；否则按「目录前缀」处理。
// 返回被改写的行数。
func (d *store) RewriteBlobPrefix(oldRel, newRel string) (int, error) {
	oldRel = strings.TrimSuffix(filepath.ToSlash(oldRel), "/")
	newRel = strings.TrimSuffix(filepath.ToSlash(newRel), "/")
	if oldRel == "" || oldRel == newRel {
		return 0, nil
	}
	rows, err := d.db.Query(`SELECT id, blob_path FROM resources WHERE blob_path = ? OR blob_path LIKE ?`,
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
