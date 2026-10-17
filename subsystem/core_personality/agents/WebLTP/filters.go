package WebLTP

// Web-LTP 结果过滤与 URL 规范化：
//   - 字典站过滤：词典/字典类站点在检索情报场景下无价值，直接从结果中剔除；
//   - 工具站兜底检测：搜索引擎对口语化查询常返回快递/whois/学信网等固定工具站，
//     当 ≥2/3 结果命中工具站特征时判定该引擎本轮失效，交由引擎降级链换引擎重试；
//   - URL 规范化与去重：协议/主机大小写、尾部斜杠、#fragment 差异视为同一页面。

import (
	"LunarSubsystem/LoggerGeneral"
	"net/url"
	"strings"
)

// dictionaryKeywords 字典/词典类网站关键词黑名单
var dictionaryKeywords = []string{
	"字典", "词典", "辞典", "dictionary", "lexicon", "thesaurus",
}

// toolSiteMarkers 工具站兜底特征词（快递/物流/在线工具/whois/学信网/地图等，匹配前统一转小写）
var toolSiteMarkers = []string{
	"快递", "物流", "运单", "货运", "kuaidi", "ickd", "zto",
	"17track", "ip地址", "在线工具", "whois", "域名查询",
	"站长工具", "学信网", "chsi", "map.baidu", "爱企查", "工商查询",
}

// filterDictionarySites 剔除标题/URL 命中字典关键词的结果
func filterDictionarySites(results []SearchHit) []SearchHit {
	if len(results) == 0 {
		return results
	}
	kept := make([]SearchHit, 0, len(results))
	for _, r := range results {
		blob := strings.ToLower(r.Title + " " + r.URL)
		dict := false
		for _, kw := range dictionaryKeywords {
			if strings.Contains(blob, kw) {
				dict = true
				break
			}
		}
		if dict {
			LoggerGeneral.Info("CrystalAstral", "Web-LTP 过滤字典站结果: %s", truncateRunes(r.URL, 80))
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// detectToolFallback 判断结果是否为搜索引擎的工具站兜底页（≥2/3 结果命中工具站特征）
func detectToolFallback(results []SearchHit) bool {
	if len(results) == 0 {
		return false
	}
	hit := 0
	for _, r := range results {
		blob := strings.ToLower(r.Title + " " + r.URL + " " + r.Snippet)
		for _, m := range toolSiteMarkers {
			if strings.Contains(blob, m) {
				hit++
				break
			}
		}
	}
	return hit*3 >= len(results)*2
}

// normalizeURL 规范化 URL（小写协议与主机、去掉 #fragment 与尾部斜杠），用作去重键与缓存键
func normalizeURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}
	return parsed.String()
}

// isRedirectWrapper 疑似搜索引擎跳转链接（baidu/link?url=、sogou/link?url= 等）。
// 这类链接的地址参数随结果页刷新变化，不适合作为缓存键。
func isRedirectWrapper(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(parsed.Path), "/link")
}

// dedupeResults 按规范化 URL 去重，保持原顺序
func dedupeResults(results []SearchHit) []SearchHit {
	seen := make(map[string]bool, len(results))
	kept := make([]SearchHit, 0, len(results))
	for _, r := range results {
		key := normalizeURL(r.URL)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, r)
	}
	return kept
}
