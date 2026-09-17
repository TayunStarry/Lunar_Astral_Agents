package WebLTP

// Web-LTP 页面摘要缓存 — SQLite（github.com/mattn/go-sqlite3，与 StarLTP 同驱动）。
// 目的：同一网页的情报只摘要一次，后续检索直接复用，减少多模态摘要调用消耗。
// 策略：以规范化 URL 为键存「标题/域名/摘要/检索词提示/入库时间」；
//   - 命中且未超过 TTL（默认 7 天）→ 跳过访问与摘要，直接复用；
//   - 命中但超过 TTL → 视为未命中，照常访问网页并用新摘要覆写旧行；
//   - 未命中 → 访问并写入。
// 打开失败或摘要为空/过短的页面不写缓存。数据库统一落在共享知识库
// GeneralConfig.KnowledgeDBPath（默认 LocalDir/database/knowledge.db，表 web_ltp_page_cache），
// 不保留旧版本缓存存储的迁移策略。

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"
	_ "github.com/mattn/go-sqlite3"
)


// pageCachePath 缓存数据库路径（与共享知识库同库：GeneralConfig.KnowledgeDBPath，
// 默认 LocalDir/database/knowledge.db）
func pageCachePath() string {
	return *GeneralConfig.KnowledgeDBPath
}

// openPageCache 打开（必要时创建）缓存数据库
func openPageCache(path string, ttl time.Duration) (*PageCache, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("打开缓存数据库失败: %w", err)
	}
	const schema = `CREATE TABLE IF NOT EXISTS web_ltp_page_cache (
		url        TEXT PRIMARY KEY,
		title      TEXT NOT NULL DEFAULT '',
		domain     TEXT NOT NULL DEFAULT '',
		summary    TEXT NOT NULL DEFAULT '',
		query_hint TEXT NOT NULL DEFAULT '',
		fetched_at INTEGER NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化缓存表失败: %w", err)
	}
	return &PageCache{db: db, ttl: ttl}, nil
}

// get 取一条缓存；命中且未过期返回 true（过期视为未命中，由调用方重新访问并覆写）
func (c *PageCache) get(rawURL string) (pageCacheEntry, bool) {
	entry := pageCacheEntry{}
	if c == nil || c.db == nil || strings.TrimSpace(rawURL) == "" {
		return entry, false
	}
	var fetchedAt int64
	err := c.db.QueryRow(`SELECT title, domain, summary, query_hint, fetched_at
		FROM web_ltp_page_cache WHERE url = ?`, rawURL).
		Scan(&entry.Title, &entry.Domain, &entry.Summary, &entry.QueryHint, &fetchedAt)
	if err != nil {
		return entry, false // 含 sql.ErrNoRows
	}
	entry.FetchedAt = time.Unix(fetchedAt, 0)
	if time.Since(entry.FetchedAt) >= c.ttl || strings.TrimSpace(entry.Summary) == "" {
		return entry, false
	}
	return entry, true
}

// put 覆写（或插入）一条缓存记录；摘要过短或为占位文本时不入库
func (c *PageCache) put(rawURL, title, domain, summary, queryHint string) {
	if c == nil || c.db == nil || strings.TrimSpace(rawURL) == "" {
		return
	}
	if len([]rune(strings.TrimSpace(summary))) < 20 || strings.Contains(summary, "页面无可提取文本") {
		return
	}
	_, err := c.db.Exec(`INSERT INTO web_ltp_page_cache (url, title, domain, summary, query_hint, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			domain = excluded.domain,
			summary = excluded.summary,
			query_hint = excluded.query_hint,
			fetched_at = excluded.fetched_at`,
		rawURL, title, domain, summary, queryHint, time.Now().Unix())
	if err != nil {
		LoggerGeneralWarn("页面摘要缓存写入失败 %s: %v", truncateRunes(rawURL, 80), err)
	}
}

// close 关闭数据库连接
func (c *PageCache) close() {
	if c != nil && c.db != nil {
		c.db.Close()
	}
}
