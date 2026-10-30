package WebSearch

// cache.go 页面「近期已访问」标记 —— 统一读写句柄库的 URL 主表
//
// 统一句柄库的 url_master 同时承担：
//   - 用户/渠道链接的 URL ⇄ 解析文本唯一绑定；
//   - 网络检索详情页的摘要缓存（不占句柄）。
// 本包只通过 engine/resource 的接口读写，不再直接操作 web.db。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// openPageCache 确认句柄库可用并返回缓存视图
func openPageCache(ttl time.Duration) (*PageCache, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	if err := resource.EnsureReady(); err != nil {
		return nil, fmt.Errorf("打开统一句柄库失败: %w", err)
	}
	return &PageCache{ttl: ttl}, nil
}

// fresh 判断某个 URL 是否在近期访问窗口内
func (c *PageCache) fresh(rawURL string) bool {
	if c == nil {
		return false
	}
	return resource.WebPageFresh(rawURL, c.ttl)
}

// put 写入（或覆写）一条访问记录
func (c *PageCache) put(rawURL, title, _, summary string) {
	if c == nil {
		return
	}
	if len([]rune(summary)) < 20 {
		return
	}
	if err := resource.RememberWebPage(rawURL, title, summary); err != nil {
		loggerWarn("页面访问记录写入失败 %s: %v", truncateRunes(rawURL, 80), err)
	}
}

// close 释放引用（连接归句柄库管理）
func (c *PageCache) close() {}
