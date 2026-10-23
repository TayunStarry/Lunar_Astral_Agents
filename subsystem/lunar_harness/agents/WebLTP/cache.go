package WebLTP

// Web-LTP 页面摘要缓存 — SQLite（与共享存储层同驱动）。
// 目的：同一网页的情报只摘要一次，后续检索直接复用，减少多模态摘要调用消耗。
// 策略：以规范化 URL 为键存「标题/域名/摘要/检索词提示/入库时间」；
//   - 命中且未超过 TTL（默认 7 天）→ 跳过访问与摘要，直接复用；
//   - 命中但超过 TTL → 视为未命中，照常访问网页并用新摘要覆写旧行；
//   - 未命中 → 访问并写入。
// 打开失败或摘要为空/过短的页面不写缓存。数据库落在网络检索领域库
// config.WebDBPath（默认 LocalDir/database/web.db，表 web_ltp_page_cache），
// 与知识库 knowledge.db 分离；表结构由 persistence 领域库统一建好，本包不重复建表。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/persistence"
)

// openPageCache 取得网络检索缓存库连接（懒打开、幂等建表）。
// 连接由 persistence 统一持有，PageCache 只借用，不负责关闭。
func openPageCache(ttl time.Duration) (*PageCache, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	db, err := persistence.EnsureWebDB()
	if err != nil {
		return nil, fmt.Errorf("打开网络检索缓存数据库失败: %w", err)
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
		loggerWarn("页面摘要缓存写入失败 %s: %v", truncateRunes(rawURL, 80), err)
	}
}

// close 释放句柄引用（连接归 persistence 领域库管理，不在此关闭）
func (c *PageCache) close() {
	if c != nil {
		c.db = nil
	}
}
