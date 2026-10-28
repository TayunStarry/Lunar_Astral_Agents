package knowledge

// web_cache.go 网页检索缓存的读写 —— 网络检索负责写入，归档层只读补摘要。
//
// web_ltp_page_cache 以规范化 URL 为键记录标题 / 域名 / 摘要 / 入库时间。
// 表结构与「近期已访问」判定统一收在本包，调用方不再自行拼该表的 SQL。

import (
	"fmt"
	"strings"
	"time"
)

// WebPageSummary 按地址读取网页检索缓存的标题与摘要（未命中返回空串）
func WebPageSummary(rawURL string) (title, summary string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ""
	}
	db, err := EnsureWeb()
	if err != nil || db == nil {
		return "", ""
	}
	_ = db.QueryRow(`SELECT title, summary FROM web_ltp_page_cache WHERE url = ?`, rawURL).
		Scan(&title, &summary)
	return strings.TrimSpace(title), strings.TrimSpace(summary)
}

// WebCacheFresh 判断地址是否在近期访问窗口内：命中、摘要有内容且距入库时间未超过 ttl。
// 地址为空或缓存库不可用时返回 false。
func WebCacheFresh(rawURL string, ttl time.Duration) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	db, err := EnsureWeb()
	if err != nil || db == nil {
		return false
	}
	var summary string
	var fetchedAt int64
	if err := db.QueryRow(`SELECT summary, fetched_at FROM web_ltp_page_cache WHERE url = ?`, rawURL).
		Scan(&summary, &fetchedAt); err != nil {
		return false // 含 sql.ErrNoRows
	}
	if strings.TrimSpace(summary) == "" {
		return false
	}
	return time.Since(time.Unix(fetchedAt, 0)) < ttl
}

// PutWebCache 写入（或覆写）一条网页检索缓存记录；地址为空、摘要过短或为占位文本时跳过。
func PutWebCache(rawURL, title, domain, summary string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	if len([]rune(strings.TrimSpace(summary))) < 20 || strings.Contains(summary, "页面无可提取文本") {
		return nil
	}
	db, err := EnsureWeb()
	if err != nil {
		return fmt.Errorf("打开网络检索缓存数据库失败: %w", err)
	}
	_, err = db.Exec(`INSERT INTO web_ltp_page_cache (url, title, domain, summary, fetched_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			domain = excluded.domain,
			summary = excluded.summary,
			fetched_at = excluded.fetched_at`,
		rawURL, title, domain, summary, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("写入网页检索缓存失败: %w", err)
	}
	return nil
}
