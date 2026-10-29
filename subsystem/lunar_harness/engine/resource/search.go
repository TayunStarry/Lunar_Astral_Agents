package resource

// search.go 跨资源搜索（标题 / 解析文本 / 标签 / 文件名 / 段落标题）

import (
	"fmt"
	"strings"
)

// SearchResources 跨资源搜索：标题 / 解析文本 / 标签 / 文件名 / 段落标题 子串匹配
func (d *store) SearchResources(query string, kind string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	like := "%" + query + "%"
	cond := `bound_handle <> '' AND (title LIKE ? OR parse_text LIKE ? OR tags LIKE ? OR name LIKE ?)`
	args := []any{like, like, like, like}
	sqlText := `SELECT ` + urlMasterColumns + ` FROM url_master WHERE ` + cond
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
	seen := map[string]bool{}
	for rows.Next() {
		res, scanErr := scanURLRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		seen[res.URL] = true
		hits = append(hits, SearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.ParseText, MatchedIn: "card",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 段落标题命中：按 URL 聚合到一条命中上
	secSQL := `SELECT s.url, s.ordinal, s.level, s.title, s.path, s.line_start, s.line_end, s.chars
		FROM doc_sections s WHERE s.title LIKE ? OR s.path LIKE ?`
	secArgs := []any{like, like}
	if kind != "" {
		secSQL += ` AND s.url IN (SELECT url FROM url_master WHERE kind = ?)`
		secArgs = append(secArgs, kind)
	}
	secSQL += ` ORDER BY s.url DESC, s.ordinal LIMIT ?`
	secArgs = append(secArgs, limit*4)
	secRows, err := d.db.Query(secSQL, secArgs...)
	if err != nil {
		return hits, nil
	}
	defer secRows.Close()
	byURL := map[string][]Section{}
	order := []string{}
	for secRows.Next() {
		var rawURL string
		var s Section
		if scanErr := secRows.Scan(&rawURL, &s.Ordinal, &s.Level, &s.Title, &s.Path,
			&s.LineStart, &s.LineEnd, &s.Chars); scanErr != nil {
			continue
		}
		s.ID = int64(s.Ordinal)
		if !seen[rawURL] {
			seen[rawURL] = true
			order = append(order, rawURL)
		}
		byURL[rawURL] = append(byURL[rawURL], s)
	}
	for _, rawURL := range order {
		res, ok := d.findURLRecord(rawURL)
		if !ok || res.Handle == "" {
			continue
		}
		hits = append(hits, SearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.ParseText, Sections: byURL[rawURL], MatchedIn: "section",
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
