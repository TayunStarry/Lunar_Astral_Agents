package resource

// handles.go 六张句柄表的唯一读写入口 + 统一入库写入
//
// 句柄表只存 (seq, handle, url)：类型由表名表达，元数据与解析文本在 url_master。
// 任何句柄分配都必须经过 InsertResource，写入句柄行与 url_master.bound_handle 在同一事务内完成。

import (
	"fmt"
	"strings"
	"time"
)

// handleKindOrder 句柄表的固定遍历顺序
var handleKindOrder = []string{KindDocument, KindImage, KindAudio, KindVideo, KindAnimated, KindURL}

// handleTable 类型码 → 句柄表名（未知类型返回空串）
func handleTable(kind string) string {
	switch kind {
	case KindDocument:
		return "handle_file"
	case KindImage:
		return "handle_image"
	case KindAudio:
		return "handle_audio"
	case KindVideo:
		return "handle_video"
	case KindAnimated:
		return "handle_animated"
	case KindURL:
		return "handle_web"
	}
	return ""
}

// InsertResource 写入一条资源并分配句柄
//
// 同一 URL 已有句柄时直接复用（不改写解析文本）；类型码变化则作废旧句柄重新分配。
func (d *store) InsertResource(res *Resource) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()

	if handleTable(res.Kind) == "" {
		return fmt.Errorf("未知类型码: %q", res.Kind)
	}
	if res.URL == "" && res.BlobPath != "" {
		res.URL = FileReadURL(res.BlobPath)
	}
	res.URL = NormalizeURL(res.URL)
	if res.URL == "" {
		return fmt.Errorf("资源 URL 不能为空")
	}
	if res.BlobPath == "" {
		res.BlobPath = RelFromURL(res.URL)
	}
	now := time.Now().Unix()
	if res.CreatedAt == 0 {
		res.CreatedAt = now
	}
	if res.UsedAt == 0 {
		res.UsedAt = now
	}
	if res.Retain == "" {
		res.Retain = DefaultRetain(res.Origin)
	}
	if res.State == "" {
		res.State = StateActive
	}
	if res.ParseState == "" {
		res.ParseState = ParseNone
	}

	existing, found := d.findURLRecord(res.URL)
	if found {
		if existing.Handle != "" && existing.Kind == res.Kind {
			// 同一 URL 同一类型：复用既有句柄，仅续期
			*res = existing
			d.touchURLInternal(existing.ID)
			return nil
		}
		if existing.Handle != "" {
			// 类型码变化：旧句柄语义失效，删除后重新分配
			if err := d.deleteHandleLocked(existing.Kind, existing.Seq); err != nil {
				return err
			}
			_, _ = d.db.Exec(`UPDATE url_master SET bound_handle = '' WHERE id = ?`, existing.ID)
		}
		// 保留既有解析文本
		if res.ParseText == "" {
			res.ParseText = existing.ParseText
		}
		if res.ParseState == ParseNone && existing.ParseState != "" {
			res.ParseState = existing.ParseState
		}
	}

	seq, err := d.nextSeqLocked(res.Kind)
	if err != nil {
		return err
	}
	handle := FormatHandle(res.Kind, seq)

	tx, err := d.db.Begin()
	if err != nil {
		archiveSeqCache[res.Kind] = seq - 1
		return err
	}
	if found {
		if _, err = tx.Exec(`UPDATE url_master SET kind = ?, name = ?, ext = ?, mime = ?, content_hash = ?,
			blob_path = ?, size_bytes = ?, width = ?, height = ?, duration_ms = ?, title = ?, tags = ?,
			origin = ?, retain = ?, state = ?, section_count = ?, used_at = ?,
			parse_text = CASE WHEN parse_text = '' THEN ? ELSE parse_text END,
			parse_state = CASE WHEN parse_state = 'none' THEN ? ELSE parse_state END,
			bound_handle = ?
			WHERE id = ?`,
			res.Kind, res.Name, res.Ext, res.MIME, res.ContentHash, res.BlobPath, res.SizeBytes,
			res.Width, res.Height, res.DurationMS, res.Title, tagsToJSON(res.Tags), res.Origin,
			res.Retain, res.State, res.SectionCount, now, res.ParseText, res.ParseState,
			handle, existing.ID); err != nil {
			rollbackTx(tx)
			archiveSeqCache[res.Kind] = seq - 1
			return err
		}
		// 句柄表里可能残留同 URL 的旧行（异常数据），先清掉再写
		if _, err = tx.Exec(`DELETE FROM `+handleTable(res.Kind)+` WHERE url = ?`, res.URL); err != nil {
			rollbackTx(tx)
			archiveSeqCache[res.Kind] = seq - 1
			return err
		}
		if _, err = tx.Exec(`INSERT INTO `+handleTable(res.Kind)+` (seq, handle, url) VALUES (?, ?, ?)`,
			seq, handle, res.URL); err != nil {
			rollbackTx(tx)
			archiveSeqCache[res.Kind] = seq - 1
			return err
		}
		if err = tx.Commit(); err != nil {
			archiveSeqCache[res.Kind] = seq - 1
			return err
		}
		res.ID = existing.ID
		res.Seq = seq
		res.Handle = handle
		return nil
	}

	res.Seq = seq
	res.Handle = handle
	result, err := tx.Exec(`INSERT INTO url_master
		(url, parse_text, parse_state, bound_handle, kind, name, ext, mime, content_hash, blob_path,
		 size_bytes, width, height, duration_ms, title, tags, origin, retain, state, section_count,
		 created_at, used_at, parsed_at, blob_owned)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		res.URL, res.ParseText, res.ParseState, handle, res.Kind, res.Name, res.Ext, res.MIME,
		res.ContentHash, res.BlobPath, res.SizeBytes, res.Width, res.Height, res.DurationMS,
		res.Title, tagsToJSON(res.Tags), res.Origin, res.Retain, res.State, res.SectionCount,
		res.CreatedAt, res.UsedAt, now, boolToInt(res.BlobOwned))
	if err != nil {
		rollbackTx(tx)
		archiveSeqCache[res.Kind] = seq - 1
		return fmt.Errorf("写入 URL 主表失败: %v", err)
	}
	if _, err = tx.Exec(`INSERT INTO `+handleTable(res.Kind)+` (seq, handle, url) VALUES (?, ?, ?)`,
		seq, handle, res.URL); err != nil {
		rollbackTx(tx)
		archiveSeqCache[res.Kind] = seq - 1
		return fmt.Errorf("写入句柄表失败: %v", err)
	}
	if err = tx.Commit(); err != nil {
		archiveSeqCache[res.Kind] = seq - 1
		return err
	}
	res.ID, _ = result.LastInsertId()
	return nil
}

// touchURLInternal 续期 URL 记录（调用方持有 archiveMu）
func (d *store) touchURLInternal(id int64) {
	_, _ = d.db.Exec(`UPDATE url_master SET used_at = ? WHERE id = ?`, time.Now().Unix(), id)
}

// handleURL 按类型码与序号读取句柄对应的 URL
func (d *store) handleURL(kind string, seq int64) (string, bool) {
	table := handleTable(kind)
	if table == "" || seq <= 0 {
		return "", false
	}
	var rawURL string
	if err := d.db.QueryRow(`SELECT url FROM `+table+` WHERE seq = ?`, seq).Scan(&rawURL); err != nil {
		return "", false
	}
	return rawURL, true
}

// findHandleByURL 在六张句柄表中定位占用某 URL 的句柄
func (d *store) findHandleByURL(rawURL string) (kind, handle string, ok bool) {
	for _, candidate := range handleKindOrder {
		var found string
		err := d.db.QueryRow(`SELECT handle FROM `+handleTable(candidate)+` WHERE url = ?`, rawURL).Scan(&found)
		if err == nil {
			return candidate, found, true
		}
	}
	return "", "", false
}

// deleteHandleLocked 删除一条句柄行（调用方持有 archiveMu）
func (d *store) deleteHandleLocked(kind string, seq int64) error {
	table := handleTable(kind)
	if table == "" {
		return nil
	}
	_, err := d.db.Exec(`DELETE FROM `+table+` WHERE seq = ?`, seq)
	return err
}

// handleRow 一条句柄映射
type handleRow struct {
	Kind   string
	Seq    int64
	Handle string
	URL    string
}

// listHandleRows 列出某类型（或全部）的句柄映射，按序号倒序
func (d *store) listHandleRows(kind string, limit int) ([]handleRow, error) {
	if limit <= 0 {
		limit = 50
	}
	kinds := handleKindOrder
	if kind != "" {
		if handleTable(kind) == "" {
			return nil, fmt.Errorf("未知类型码: %q", kind)
		}
		kinds = []string{kind}
	}
	parts := make([]string, 0, len(kinds))
	args := make([]any, 0, len(kinds)*2)
	for _, candidate := range kinds {
		parts = append(parts, `SELECT ? AS kind, seq, handle, url FROM `+handleTable(candidate))
		args = append(args, candidate)
	}
	query := strings.Join(parts, " UNION ALL ") + ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []handleRow{}
	for rows.Next() {
		var row handleRow
		if scanErr := rows.Scan(&row.Kind, &row.Seq, &row.Handle, &row.URL); scanErr == nil {
			out = append(out, row)
		}
	}
	return out, rows.Err()
}

// kindFromHandle 从规范化句柄文本取类型码
func kindFromHandle(raw string) string {
	if handle, ok := ParseHandle(raw); ok {
		return handle.Kind
	}
	return ""
}

// trimKind 大写化并校验类型码
func trimKind(kind string) string {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if handleTable(kind) == "" {
		return ""
	}
	return kind
}
