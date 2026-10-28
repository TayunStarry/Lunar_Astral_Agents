package resource

// query.go 资源目录的读取与写入原语：按句柄 / 路径 / 哈希 / 链接定位，
// 列表与段落表维护；句柄分配仍由 store.go 的 InsertResource 统一完成。

import (
	"encoding/json"
	"strings"
	"time"
)

// GetResourceByHandle 按句柄解析资源（句柄不存在时返回 false）
func (d *store) GetResourceByHandle(kind string, seq int64) (Resource, bool) {
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+` FROM resources WHERE kind = ? AND seq = ?`, kind, seq)
	res, err := scanResource(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// GetResourceByID 按自增 ID 解析资源
func (d *store) GetResourceByID(id int64) (Resource, bool) {
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+` FROM resources WHERE id = ?`, id)
	res, err := scanResource(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// FindResourceByBlobPath 按实体路径查找既有目录行（用于沿用实体归属）
func (d *store) FindResourceByBlobPath(blobPath string) (Resource, bool) {
	if blobPath == "" {
		return Resource{}, false
	}
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE blob_path = ? ORDER BY id ASC LIMIT 1`, blobPath)
	res, err := scanResource(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// FindResourceByHash 按类型码与内容哈希查找既有资源（用于同名同内容复用）
func (d *store) FindResourceByHash(kind, hash string) (Resource, bool) {
	if hash == "" {
		return Resource{}, false
	}
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE kind = ? AND content_hash = ? ORDER BY id DESC LIMIT 1`, kind, hash)
	res, err := scanResource(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// ListResources 列出资源（kind 为空表示全部），按 id 倒序（越靠前越新）
func (d *store) ListResources(kind string, limit int) ([]Resource, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + archiveResourceColumns + ` FROM resources`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(query, args...)
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

// ListSections 读取资源的段落表（按章节序号升序）
func (d *store) ListSections(resourceID int64) ([]Section, error) {
	rows, err := d.db.Query(`SELECT id, resource_id, ordinal, level, title, path,
		line_start, line_end, chars FROM resource_sections WHERE resource_id = ? ORDER BY ordinal`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Section{}
	for rows.Next() {
		var s Section
		if err := rows.Scan(&s.ID, &s.Resource, &s.Ordinal, &s.Level, &s.Title, &s.Path,
			&s.LineStart, &s.LineEnd, &s.Chars); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReplaceSections 全量替换资源的段落表（重导 / 迁移重建时使用）
func (d *store) ReplaceSections(resourceID int64, drafts []SectionDraft) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM resource_sections WHERE resource_id = ?`, resourceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO resource_sections
		(resource_id, ordinal, level, title, path, line_start, line_end, chars) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, s := range drafts {
		if _, err := stmt.Exec(resourceID, s.Ordinal, s.Level, s.Title, s.Path, s.LineStart, s.LineEnd, s.Chars); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE resources SET section_count = ? WHERE id = ?`, len(drafts), resourceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// FindResourceByURL 按链接地址查找既有 @U 资源（同一链接复用同一句柄）
func (d *store) FindResourceByURL(rawURL string) (Resource, bool) {
	if strings.TrimSpace(rawURL) == "" {
		return Resource{}, false
	}
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE kind = ? AND url = ? ORDER BY id ASC LIMIT 1`, KindURL, strings.TrimSpace(rawURL))
	res, err := scanResource(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// UpdateResourceURLExtras 更新链接资源的标题与摘要（网页缓存 / 视频观影感受回填）
func (d *store) UpdateResourceURLExtras(id int64, title, summary string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET title = ?, summary = ?, used_at = ? WHERE id = ?`,
		title, summary, time.Now().Unix(), id)
	return err
}

// UpdateResourceExtras 更新资源的摘要/标题/标签（描述生成与迁移回填使用）
func (d *store) UpdateResourceExtras(id int64, title, summary string, tags []string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	tagsJSON, _ := json.Marshal(tags)
	if tags == nil {
		tagsJSON = []byte("[]")
	}
	_, err := d.db.Exec(`UPDATE resources SET title = ?, summary = ?, tags = ? WHERE id = ?`,
		title, summary, string(tagsJSON), id)
	return err
}

// UpdateResourceMeta 更新资源的尺寸/时长等探测元数据
func (d *store) UpdateResourceMeta(id int64, width, height int, durationMS int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET width = ?, height = ?, duration_ms = ? WHERE id = ?`,
		width, height, durationMS, id)
	return err
}

// RefreshResourceContent 刷新资源的正文元数据（文本类按原文件名稳定复用的重复入库）
//
// 只更新与文件内容相关的列，保留标题、摘要、标签等人工/模型产物。
func (d *store) RefreshResourceContent(id int64, name, ext, mime, hash string, size int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET name = ?, ext = ?, mime = ?, content_hash = ?,
		size_bytes = ?, state = ?, used_at = ? WHERE id = ?`,
		name, ext, mime, hash, size, StateActive, time.Now().Unix(), id)
	return err
}

// TouchResource 记录资源最近被取用（GC 的 LRU 依据）
func (d *store) TouchResource(id int64) {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, _ = d.db.Exec(`UPDATE resources SET used_at = ? WHERE id = ?`, time.Now().Unix(), id)
}

// MarkState 更新资源 blob 状态（回收后置 lost，句柄保留）
func (d *store) MarkState(id int64, state string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET state = ? WHERE id = ?`, state, id)
	return err
}

// DeleteResource 显式删除资源目录行与其段落（用户主动删除时使用）
func (d *store) DeleteResource(id int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	if _, err := d.db.Exec(`DELETE FROM resource_sections WHERE resource_id = ?`, id); err != nil {
		return err
	}
	_, err := d.db.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}
