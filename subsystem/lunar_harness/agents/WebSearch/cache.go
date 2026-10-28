package WebSearch

// cache.go 页面「近期已访问」标记 —— 读写统一走 engine/knowledge 的网络检索缓存接口。
// 目的：近期访问过的网页不重复打开，本轮搜索只进入近期未访问过的链接。
// 策略：以规范化 URL 为键记录标题/域名/摘要/入库时间；
//   - 命中且未超过 TTL（默认 7 天）→ 判定为「近期已访问」，跳过该链接；
//   - 超过 TTL 或未命中 → 照常访问并覆写记录。
// 数据库落在网络检索领域库 config.WebDBPath（默认 LocalDir/database/web.db，表 web_ltp_page_cache），
// 表结构与写入判定由 knowledge 统一收口，本包不再直接拼该表的 SQL。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/knowledge"
)

// openPageCache 确认网络检索缓存可用（库连接与表结构由 knowledge 统一持有）。
// PageCache 只借用接口，不负责关闭连接。
func openPageCache(ttl time.Duration) (*PageCache, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	if _, err := knowledge.EnsureWeb(); err != nil {
		return nil, fmt.Errorf("打开网络检索缓存数据库失败: %w", err)
	}
	return &PageCache{ttl: ttl}, nil
}

// fresh 判断某个 URL 是否在近期访问窗口内（命中且未过期返回 true）
func (c *PageCache) fresh(rawURL string) bool {
	if c == nil {
		return false
	}
	return knowledge.WebCacheFresh(rawURL, c.ttl)
}

// put 写入（或覆写）一条访问记录；摘要过短或为占位文本时不入库
func (c *PageCache) put(rawURL, title, domain, summary string) {
	if c == nil {
		return
	}
	if err := knowledge.PutWebCache(rawURL, title, domain, summary); err != nil {
		loggerWarn("页面访问记录写入失败 %s: %v", truncateRunes(rawURL, 80), err)
	}
}

// close 释放引用（连接归 knowledge 领域库管理，不在此关闭）
func (c *PageCache) close() {}
