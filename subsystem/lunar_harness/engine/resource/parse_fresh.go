package resource

// parse_fresh.go 解析文本有效期判定（全仓唯一实现）
//
// 分档口径：
//   - 网页 / 搜索结果：7 天（WebParseTTL）
//   - B 站视频直驱印象：30 天（BilibiliParseTTL）
//   - 本地文件 / 本地视频：不设 TTL，内容变化由 content_hash 指纹触发失效
//
// 过期即视为未命中：调用方必须走重新生成路径并覆盖旧文本，
// 不允许返回过期内容，也不做「先返回旧的再后台刷新」。

import (
	"strings"
	"time"
)

// ParseCategoryOf 按资源属性判定内容类别（唯一分类实现）。
//
// B 站只认视频 BV：专栏、番剧页等没有 BV 的地址按网页档处理；
// 本地实体（有 BlobPath）不设 TTL。
func ParseCategoryOf(res Resource) ParseCategory {
	if strings.TrimSpace(res.BlobPath) != "" {
		return ParseCategoryLocal
	}
	if BilibiliBvid(res.URL) != "" {
		return ParseCategoryBilibili
	}
	return ParseCategoryWeb
}

// ParseTextFresh 判断解析文本是否仍可用（唯一对外判定入口）。
func ParseTextFresh(res Resource, category ParseCategory) bool {
	return parseTextFreshWithin(res, parseTTL(category))
}

// parseTTL 类别 → 有效期；零值表示永不过期。
func parseTTL(category ParseCategory) time.Duration {
	switch category {
	case ParseCategoryBilibili:
		return BilibiliParseTTL
	case ParseCategoryLocal:
		return 0
	default:
		return WebParseTTL
	}
}

// parseTextFreshWithin 唯一实现：状态、文本与写入时间三者的过期比较都只在这里。
func parseTextFreshWithin(res Resource, ttl time.Duration) bool {
	if res.ParseState != ParseReady || strings.TrimSpace(res.ParseText) == "" {
		return false
	}
	if ttl <= 0 {
		return true
	}
	if res.ParsedAt <= 0 {
		return false
	}
	return time.Since(time.Unix(res.ParsedAt, 0)) < ttl
}
