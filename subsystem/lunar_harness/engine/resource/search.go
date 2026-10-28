package resource

// search.go 跨资源目录搜索（标题 / 摘要 / 标签 / 文件名 / 段落标题）。

import (
	"fmt"
	"strings"
)

// SearchResources 跨资源目录搜索：标题 / 摘要 / 标签 / 文件名 / 段落标题 子串匹配
//
// 说明：刻意不使用 FTS5 —— 中文无空格分词在 unicode61 下切分不可用，
// 且资源目录规模有限（千级），子串匹配的召回率与确定性优于虚表。
func (d *store) SearchResources(query string, kind string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	like := "%" + query + "%"
	cond := `(title LIKE ? OR summary LIKE ? OR tags LIKE ? OR name LIKE ?)`
	args := []any{like, like, like, like}
	sqlText := `SELECT ` + archiveResourceColumns + ` FROM resources WHERE ` + cond
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
	hits := []SearchHit{}
	seen := map[int64]bool{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		seen[res.ID] = true
		hits = append(hits, SearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.Summary, MatchedIn: "card",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 段落标题命中：文档按章节补齐（同一资源聚合到一条命中上）
	secSQL := `SELECT s.resource_id, s.id, s.resource_id, s.ordinal, s.level, s.title, s.path,
		s.line_start, s.line_end, s.chars FROM resource_sections s WHERE s.title LIKE ? OR s.path LIKE ?`
	secArgs := []any{like, like}
	if kind != "" {
		secSQL += ` AND s.resource_id IN (SELECT id FROM resources WHERE kind = ?)`
		secArgs = append(secArgs, kind)
	}
	secSQL += ` ORDER BY s.resource_id DESC, s.ordinal LIMIT ?`
	secArgs = append(secArgs, limit*4)
	secRows, err := d.db.Query(secSQL, secArgs...)
	if err != nil {
		return hits, nil
	}
	defer secRows.Close()
	byResource := map[int64][]Section{}
	order := []int64{}
	for secRows.Next() {
		var resID int64
		var s Section
		if err := secRows.Scan(&resID, &s.ID, &s.Resource, &s.Ordinal, &s.Level, &s.Title,
			&s.Path, &s.LineStart, &s.LineEnd, &s.Chars); err != nil {
			continue
		}
		if !seen[resID] {
			seen[resID] = true
			order = append(order, resID)
		}
		byResource[resID] = append(byResource[resID], s)
	}
	for _, resID := range order {
		res, ok := d.GetResourceByID(resID)
		if !ok {
			continue
		}
		hits = append(hits, SearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.Summary, Sections: byResource[resID], MatchedIn: "section",
		})
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// displayTitle 资源的展示标题（标题为空时退回文件名，再退回句柄）
func displayTitle(res Resource) string {
	if res.Title != "" {
		return res.Title
	}
	if res.Name != "" {
		return res.Name
	}
	return res.Handle
}
