package resource

// query.go URL 主表与句柄表的查询 / 更新原语
//
// 所有句柄查询都是点查：句柄表 seq/url 唯一索引 → url_master.url 唯一索引，不做线性扫描。

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GetResourceByHandle 按类型码 + 序号解析资源（不存在返回 false）
func (d *store) GetResourceByHandle(kind string, seq int64) (Resource, bool) {
	rawURL, ok := d.handleURL(kind, seq)
	if !ok {
		return Resource{}, false
	}
	return d.findURLRecord(rawURL)
}

// GetResourceByID 按 url_master 主键解析资源
func (d *store) GetResourceByID(id int64) (Resource, bool) {
	row := d.db.QueryRow(`SELECT `+urlMasterColumns+` FROM url_master WHERE id = ?`, id)
	res, err := scanURLRecord(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// FindResourceByBlobPath 按内容仓库相对路径解析资源
func (d *store) FindResourceByBlobPath(rel string) (Resource, bool) {
	return d.findURLRecord(FileReadURL(rel))
}

// FindResourceByHash 按类型码 + 内容哈希解析已绑定句柄的资源
func (d *store) FindResourceByHash(kind, hash string) (Resource, bool) {
	if hash == "" {
		return Resource{}, false
	}
	row := d.db.QueryRow(`SELECT `+urlMasterColumns+` FROM url_master
		WHERE kind = ? AND content_hash = ? AND bound_handle <> '' ORDER BY id DESC LIMIT 1`, kind, hash)
	res, err := scanURLRecord(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// ListResources 列出已绑定句柄的资源（kind 为空表示全部），按最近使用倒序
func (d *store) ListResources(kind string, limit int) ([]Resource, error) {
	rows, err := d.listHandleRows(kind, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Resource, 0, len(rows))
	for _, row := range rows {
		res, ok := d.findURLRecord(row.URL)
		if !ok {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// ListResourcesMatching 按文件名 / 路径 / 地址 / 标题子串过滤资源（kind 为空表示全部）
//
// keywords 之间取并集：任一关键词命中即列出，便于「文件名」「纯文件夹名」「相对路径」混用。
func (d *store) ListResourcesMatching(kind string, keywords []string, limit int) ([]Resource, error) {
	if limit <= 0 {
		limit = 50
	}
	conds := make([]string, 0, len(keywords))
	args := make([]any, 0, len(keywords)*4+1)
	for _, raw := range keywords {
		keyword := strings.TrimSpace(raw)
		if keyword == "" {
			continue
		}
		like := "%" + keyword + "%"
		conds = append(conds, `(name LIKE ? OR blob_path LIKE ? OR url LIKE ? OR title LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if len(conds) == 0 {
		return d.ListResources(kind, limit)
	}
	sqlText := `SELECT ` + urlMasterColumns + ` FROM url_master WHERE bound_handle <> '' AND (` + strings.Join(conds, " OR ") + `)`
	if kind != "" {
		sqlText += ` AND kind = ?`
		args = append(args, kind)
	}
	sqlText += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(sqlText, args...)
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

// ListSections 读取资源段落表（按章节序号升序）
func (d *store) ListSections(resourceID int64) ([]Section, error) {
	res, ok := d.GetResourceByID(resourceID)
	if !ok {
		return nil, nil
	}
	return d.ListSectionsByURL(res.URL)
}

// ListSectionsByURL 按 URL 读取段落表
func (d *store) ListSectionsByURL(rawURL string) ([]Section, error) {
	rawURL = NormalizeURL(rawURL)
	rows, err := d.db.Query(`SELECT ordinal, level, title, path, line_start, line_end, chars
		FROM doc_sections WHERE url = ? ORDER BY ordinal`, rawURL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Section{}
	for rows.Next() {
		var s Section
		s.Resource = 0
		s.ID = int64(s.Ordinal)
		if err := rows.Scan(&s.Ordinal, &s.Level, &s.Title, &s.Path, &s.LineStart, &s.LineEnd, &s.Chars); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReplaceSections 全量替换资源段落表
func (d *store) ReplaceSections(resourceID int64, drafts []SectionDraft) error {
	res, ok := d.GetResourceByID(resourceID)
	if !ok {
		return fmt.Errorf("资源不存在: %d", resourceID)
	}
	return d.ReplaceSectionsByURL(res.URL, drafts)
}

// ReplaceSectionsByURL 按 URL 全量替换段落表
func (d *store) ReplaceSectionsByURL(rawURL string, drafts []SectionDraft) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	rawURL = NormalizeURL(rawURL)
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM doc_sections WHERE url = ?`, rawURL); err != nil {
		rollbackTx(tx)
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO doc_sections
		(url, ordinal, level, title, path, line_start, line_end, chars) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		rollbackTx(tx)
		return err
	}
	defer stmt.Close()
	for _, s := range drafts {
		if _, err := stmt.Exec(rawURL, s.Ordinal, s.Level, s.Title, s.Path, s.LineStart, s.LineEnd, s.Chars); err != nil {
			rollbackTx(tx)
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE url_master SET section_count = ? WHERE url = ?`, len(drafts), rawURL); err != nil {
		rollbackTx(tx)
		return err
	}
	return tx.Commit()
}

// FindResourceByURL 按链接地址查 URL 主表记录（不要求已绑定句柄）
func (d *store) FindResourceByURL(rawURL string) (Resource, bool) {
	return d.findURLRecord(rawURL)
}

// UpdateResourceURLExtras 更新链接资源的标题与解析文本
//
// parsed_at 只在解析文本真的发生变化时续期：仅改标题不能刷新 TTL，
// 否则旧文本会被永久续命。
func (d *store) UpdateResourceURLExtras(id int64, title, summary string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	now := time.Now().Unix()
	text, state := summary, ParseNone
	if text != "" {
		state = ParseReady
	}
	_, err := d.db.Exec(`UPDATE url_master SET title = ?, parse_text = CASE WHEN ? <> '' THEN ? ELSE parse_text END,
		parse_state = CASE WHEN ? <> '' THEN ? ELSE parse_state END,
		parsed_at = CASE WHEN ? <> '' AND ? <> parse_text THEN ? ELSE parsed_at END, used_at = ? WHERE id = ?`,
		title, text, text, text, state, text, text, now, now, id)
	return err
}

// UpdateResourceExtras 更新标题 / 解析文本 / 标签
func (d *store) UpdateResourceExtras(id int64, title, summary string, tags []string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	now := time.Now().Unix()
	_, err := d.db.Exec(`UPDATE url_master SET title = ?, tags = ?,
		parse_text = CASE WHEN ? <> '' THEN ? ELSE parse_text END,
		parse_state = CASE WHEN ? <> '' THEN 'ready' ELSE parse_state END,
		parsed_at = CASE WHEN ? <> '' THEN ? ELSE parsed_at END, used_at = ?
		WHERE id = ?`,
		title, tagsToJSON(tags), summary, summary, summary, summary, now, now, id)
	return err
}

// UpdateResourceMeta 更新尺寸 / 时长
func (d *store) UpdateResourceMeta(id int64, width, height int, durationMS int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE url_master SET width = ?, height = ?, duration_ms = ? WHERE id = ?`,
		width, height, durationMS, id)
	return err
}

// RefreshResourceContent 刷新与文件内容相关的列
//
// 复用 content_hash 作为文件指纹：旧指纹非空且发生变化时，说明文件被替换，
// 旧解析文本立即失效（清空文本并置回 none），避免继续使用旧理解；
// 指纹未变或新旧任一为空时保留解析文本。
func (d *store) RefreshResourceContent(id int64, name, ext, mime, hash string, size int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE url_master SET name = ?, ext = ?, mime = ?, content_hash = ?,
		size_bytes = ?, state = ?, used_at = ?,
		parse_text = CASE WHEN content_hash <> '' AND ? <> '' AND content_hash <> ? THEN '' ELSE parse_text END,
		parse_state = CASE WHEN content_hash <> '' AND ? <> '' AND content_hash <> ? THEN ? ELSE parse_state END
		WHERE id = ?`,
		name, ext, mime, hash, size, StateActive, time.Now().Unix(),
		hash, hash, hash, hash, ParseNone, id)
	return err
}

// TouchResource 记录最近取用（GC 的 LRU 依据）
func (d *store) TouchResource(id int64) {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	d.touchURLInternal(id)
}

// MarkState 更新资源状态（回收后置 lost，句柄保留）
func (d *store) MarkState(id int64, state string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE url_master SET state = ? WHERE id = ?`, state, id)
	return err
}

// DeleteResource 删除资源：解除句柄绑定、清段落、删 URL 记录
func (d *store) DeleteResource(id int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	return d.deleteResourceLocked(id)
}

// deleteResourceLocked 删除资源（调用方持有 archiveMu）
func (d *store) deleteResourceLocked(id int64) error {
	res, ok := d.GetResourceByID(id)
	if !ok {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if res.Handle != "" {
		if _, err := tx.Exec(`DELETE FROM `+handleTable(res.Kind)+` WHERE seq = ?`, res.Seq); err != nil {
			rollbackTx(tx)
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM doc_sections WHERE url = ?`, res.URL); err != nil {
		rollbackTx(tx)
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cache WHERE source_url = ?`, res.URL); err != nil {
		rollbackTx(tx)
		return err
	}
	if res.Handle != "" {
		if _, err := tx.Exec(`DELETE FROM gc_media_score WHERE handle = ?`, res.Handle); err != nil {
			rollbackTx(tx)
			return err
		}
		if _, err := tx.Exec(`DELETE FROM gc_orphan_handle WHERE handle = ?`, res.Handle); err != nil {
			rollbackTx(tx)
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM url_master WHERE id = ?`, id); err != nil {
		rollbackTx(tx)
		return err
	}
	return tx.Commit()
}

// deleteResourceByURL 按 URL 删除（内部使用）
func (d *store) deleteResourceByURL(rawURL string) error {
	res, ok := d.findURLRecord(rawURL)
	if !ok {
		return nil
	}
	return d.DeleteResource(res.ID)
}

// queryRowResource 扫描单行（内部工具）
func (d *store) queryRowResource(query string, args ...any) (Resource, bool) {
	row := d.db.QueryRow(query, args...)
	res, err := scanURLRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Resource{}, false
		}
		return Resource{}, false
	}
	return res, true
}
