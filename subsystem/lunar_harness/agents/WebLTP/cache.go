package WebLTP

// cache.go 页面「近期已访问」标记 — SQLite（与共享存储层同驱动）。
// 目的：近期访问过的网页不重复打开，本轮搜索只进入近期未访问过的链接。
// 策略：以规范化 URL 为键记录标题/域名/摘要/入库时间；
//   - 命中且未超过 TTL（默认 7 天）→ 判定为「近期已访问」，跳过该链接；
//   - 超过 TTL 或未命中 → 照常访问并覆写记录。
// 数据库落在网络检索领域库 config.WebDBPath（默认 LocalDir/database/web.db，表 web_ltp_page_cache），
// 表结构由 persistence 领域库统一建好，本包不重复建表。

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

// fresh 判断某个 URL 是否在近期访问窗口内（命中且未过期返回 true）
func (c *PageCache) fresh(rawURL string) bool {
	if c == nil || c.db == nil || strings.TrimSpace(rawURL) == "" {
		return false
	}
	var summary string
	var fetchedAt int64
	err := c.db.QueryRow(`SELECT summary, fetched_at FROM web_ltp_page_cache WHERE url = ?`, rawURL).
		Scan(&summary, &fetchedAt)
	if err != nil {
		return false // 含 sql.ErrNoRows
	}
	if strings.TrimSpace(summary) == "" {
		return false
	}
	return time.Since(time.Unix(fetchedAt, 0)) < c.ttl
}

// put 写入（或覆写）一条访问记录；摘要过短或为占位文本时不入库
func (c *PageCache) put(rawURL, title, domain, summary string) {
	if c == nil || c.db == nil || strings.TrimSpace(rawURL) == "" {
		return
	}
	if len([]rune(strings.TrimSpace(summary))) < 20 || strings.Contains(summary, "页面无可提取文本") {
		return
	}
	_, err := c.db.Exec(`INSERT INTO web_ltp_page_cache (url, title, domain, summary, fetched_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			domain = excluded.domain,
			summary = excluded.summary,
			fetched_at = excluded.fetched_at`,
		rawURL, title, domain, summary, time.Now().Unix())
	if err != nil {
		loggerWarn("页面访问记录写入失败 %s: %v", truncateRunes(rawURL, 80), err)
	}
}

// close 释放句柄引用（连接归 persistence 领域库管理，不在此关闭）
func (c *PageCache) close() {
	if c != nil {
		c.db = nil
	}
}
