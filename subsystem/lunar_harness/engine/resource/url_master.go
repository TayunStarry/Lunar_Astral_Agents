package resource

// url_master.go 统一 URL 主表的读写：URL ⇄ 解析文本唯一绑定
//
// 约定：
//   - 本地资源的目标 URL 是服务相对路由 /file/read/<rel>（不带主机与端口）；
//   - 外部链接按 url_norm.go 的规则规范化，同一内容只留一行；
//   - 解析文本（parse_text）是这条 URL 的唯一解读结果，一旦 ready 不再重跑；
//   - bound_handle 记录占用该 URL 的句柄，偏唯一索引保证全局一对一。

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// urlMasterColumns url_master 的扫描列（与 scanURLRecord 严格对齐）
const urlMasterColumns = `id, url, parse_text, parse_state, bound_handle, kind, name, ext, mime,
	content_hash, blob_path, size_bytes, width, height, duration_ms, title, tags, origin, retain,
	state, section_count, created_at, used_at, parsed_at, blob_owned`

// scanURLRecord 按 urlMasterColumns 顺序扫描一行
func scanURLRecord(scanner interface{ Scan(...any) error }) (Resource, error) {
	var res Resource
	var tagsJSON, bound string
	var blobOwned int
	var parsedAt int64
	err := scanner.Scan(&res.ID, &res.URL, &res.ParseText, &res.ParseState, &bound, &res.Kind,
		&res.Name, &res.Ext, &res.MIME, &res.ContentHash, &res.BlobPath, &res.SizeBytes,
		&res.Width, &res.Height, &res.DurationMS, &res.Title, &tagsJSON, &res.Origin, &res.Retain,
		&res.State, &res.SectionCount, &res.CreatedAt, &res.UsedAt, &parsedAt, &blobOwned)
	if err != nil {
		return res, err
	}
	res.BlobOwned = blobOwned != 0
	res.Tags = []string{}
	// API 兼容：摘要字段直接映射唯一解析文本
	res.Summary = res.ParseText
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &res.Tags)
	}
	if bound != "" {
		if handle, ok := ParseHandle(bound); ok {
			res.Handle = FormatHandle(handle.Kind, handle.Seq)
			res.Kind = handle.Kind
			res.Seq = handle.Seq
		}
	}
	return res, nil
}

// findURLRecord 按规范化 URL 查主表记录
func (d *store) findURLRecord(raw string) (Resource, bool) {
	norm := NormalizeURL(raw)
	if norm == "" {
		return Resource{}, false
	}
	row := d.db.QueryRow(`SELECT `+urlMasterColumns+` FROM url_master WHERE url = ?`, norm)
	res, err := scanURLRecord(row)
	if err != nil {
		return Resource{}, false
	}
	return res, true
}

// upsertURLRecord 写入或用元数据刷新 URL 记录（不含句柄绑定）
//
// 已有 ready 解析文本时绝不覆盖；只有显式 parse 操作才改解析文本。
func (d *store) upsertURLRecord(res *Resource) error {
	now := time.Now().Unix()
	if res.ParseState == "" {
		res.ParseState = ParseNone
	}
	if res.State == "" {
		res.State = StateActive
	}
	if res.Retain == "" {
		res.Retain = DefaultRetain(res.Origin)
	}
	if res.CreatedAt == 0 {
		res.CreatedAt = now
	}
	if res.UsedAt == 0 {
		res.UsedAt = res.CreatedAt
	}
	existing, found := d.findURLRecord(res.URL)
	if !found {
		tagsJSON := tagsToJSON(res.Tags)
		result, err := d.db.Exec(`INSERT INTO url_master
			(url, parse_text, parse_state, kind, name, ext, mime, content_hash, blob_path, size_bytes,
			 width, height, duration_ms, title, tags, origin, retain, state, section_count,
			 created_at, used_at, parsed_at, blob_owned)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			res.URL, res.ParseText, res.ParseState, res.Kind, res.Name, res.Ext, res.MIME,
			res.ContentHash, res.BlobPath, res.SizeBytes, res.Width, res.Height, res.DurationMS,
			res.Title, tagsJSON, res.Origin, res.Retain, res.State, res.SectionCount,
			res.CreatedAt, res.UsedAt, now, boolToInt(res.BlobOwned))
		if err != nil {
			return err
		}
		res.ID, _ = result.LastInsertId()
		return nil
	}
	// 既有记录：复用 ID，刷新可变的元数据；解析文本只在空时补写
	res.ID = existing.ID
	res.ParseText = existing.ParseText
	res.ParseState = existing.ParseState
	if res.ParseState == ParseNone && strings.TrimSpace(res.ParseText) != "" {
		res.ParseState = ParseReady
	}
	// 已绑定句柄时类型由句柄表决定，主表不再改写 kind，避免检索缓存污染视频句柄
	kind := res.Kind
	if existing.Handle != "" {
		kind = existing.Kind
	}
	tagsJSON := tagsToJSON(res.Tags)
	_, err := d.db.Exec(`UPDATE url_master SET kind = ?, name = ?, ext = ?, mime = ?, content_hash = ?,
		blob_path = ?, size_bytes = ?, width = ?, height = ?, duration_ms = ?, title = ?, tags = ?,
		origin = ?, retain = ?, state = ?, section_count = ?, used_at = ?,
		parse_text = CASE WHEN parse_text = '' THEN ? ELSE parse_text END,
		parse_state = CASE WHEN parse_state = 'none' THEN ? ELSE parse_state END
		WHERE id = ?`,
		kind, res.Name, res.Ext, res.MIME, res.ContentHash, res.BlobPath, res.SizeBytes,
		res.Width, res.Height, res.DurationMS, res.Title, tagsJSON, res.Origin, res.Retain,
		res.State, res.SectionCount, now, res.ParseText, res.ParseState, existing.ID)
	if err != nil {
		return err
	}
	bound, _ := d.findURLRecord(res.URL)
	res.Handle, res.Seq = bound.Handle, bound.Seq
	return nil
}

// setURLParseText 写入唯一解析文本并置 ready（同一 URL 只留一份）
func (d *store) setURLParseText(raw, text string) error {
	norm := NormalizeURL(raw)
	if norm == "" {
		return nil
	}
	text = strings.TrimSpace(text)
	now := time.Now().Unix()
	state := ParseReady
	if text == "" {
		state = ParseFailed
	}
	_, err := d.db.Exec(`UPDATE url_master SET parse_text = ?, parse_state = ?, parsed_at = ?, used_at = ?
		WHERE url = ?`, text, state, now, now, norm)
	return err
}

// markURLParseFailed 标记解析失败（保留失败状态供人工重试）
func (d *store) markURLParseFailed(raw string) {
	norm := NormalizeURL(raw)
	if norm == "" {
		return
	}
	_, _ = d.db.Exec(`UPDATE url_master SET parse_state = ? WHERE url = ?`, ParseFailed, norm)
}

// findURLWithParseText 按关键词检索已有解析文本（记忆检索 / 视频摘要回忆使用）
func (d *store) findURLWithParseText(keywords []string, limit int) ([]Resource, error) {
	if len(keywords) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	out := []Resource{}
	seen := map[int64]bool{}
	for _, keyword := range keywords {
		like := "%" + strings.TrimSpace(keyword) + "%"
		rows, err := d.db.Query(`SELECT `+urlMasterColumns+` FROM url_master
			WHERE parse_state = ? AND (url LIKE ? OR title LIKE ? OR parse_text LIKE ?)
			ORDER BY id DESC LIMIT ?`, ParseReady, like, like, like, limit)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			res, scanErr := scanURLRecord(rows)
			if scanErr != nil {
				continue
			}
			if seen[res.ID] {
				continue
			}
			seen[res.ID] = true
			out = append(out, res)
		}
		rows.Close()
	}
	return out, nil
}

// tagsToJSON 序列化标签（nil 视为空数组）
func tagsToJSON(tags []string) string {
	raw, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// boolToInt SQLite 没有布尔类型，统一以 0/1 存取
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// rollbackTx 回滚事务（忽略错误）
func rollbackTx(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}
