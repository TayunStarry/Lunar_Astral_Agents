package websearch

import (
	"net/url"
	"strings"
)

// ScoreDomainAuthority 对URL进行权威性评分（0.0~1.0），优先使用搜索引擎官方标记
func ScoreDomainAuthority(rawURL string, isOfficial bool) float64 {
	// 搜索引擎标记的官方网站直接给满分
	if isOfficial {
		return 1.0
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return 0.5 // 无法解析时给默认分
	}

	host := strings.ToLower(u.Host)

	// 去掉 www. 前缀做统一匹配
	host = strings.TrimPrefix(host, "www.")

	// 1. 精确域名匹配：政府/教育/官方组织
	if score, ok := exactDomainScores[host]; ok {
		return score
	}

	// 2. 域名后缀匹配
	if score, ok := suffixDomainScores(host); ok {
		return score
	}

	// 3. 域名关键词匹配
	if score, ok := keywordDomainScores(host); ok {
		return score
	}

	// 4. 默认分数
	return 0.5
}

// suffixDomainScores 根据域名后缀评分
func suffixDomainScores(host string) (float64, bool) {
	suffixes := map[string]float64{
		".gov.cn": 1.0,
		".edu.cn": 0.95,
		".ac.cn":  0.9,
		".org.cn": 0.8,
		".org":    0.75,
		".edu":    0.95,
		".gov":    1.0,
		".mil":    0.95,
	}
	for suffix, score := range suffixes {
		if strings.HasSuffix(host, suffix) {
			return score, true
		}
	}
	return 0, false
}

// keywordDomainScores 根据域名关键词评分
func keywordDomainScores(host string) (float64, bool) {
	keywords := []struct {
		keyword string
		score   float64
	}{
		// 百科/知识类
		{"baike", 0.95},
		{"wiki", 0.9},
		{"encyclopedia", 0.9},

		// 知名门户/媒体
		{"sina.com", 0.7},
		{"sohu.com", 0.7},
		{"163.com", 0.7},
		{"qq.com", 0.7},
		{"people.com", 0.85},
		{"xinhuanet", 0.85},
		{"cctv.com", 0.85},
		{"chinanews", 0.8},
		{"guancha.cn", 0.75},
		{"thepaper.cn", 0.75},

		// 知名技术社区
		{"csdn.net", 0.6},
		{"zhihu.com", 0.65},
		{"jianshu.com", 0.55},
		{"segmentfault", 0.6},
		{"juejin.cn", 0.6},
		{"stackoverflow", 0.7},
		{"github.com", 0.75},
		{"gitee.com", 0.65},

		// 知名科技媒体
		{"36kr.com", 0.65},
		{"geekbang", 0.6},
		{"infoq", 0.6},
		{"oschina", 0.55},

		// 论坛/社区类
		{"tieba.baidu", 0.3},
		{"bbs.", 0.3},
		{"forum.", 0.3},
		{"douban.com", 0.5},
		{"v2ex.com", 0.5},
		{"nga.cn", 0.35},

		// 电商/商业类
		{"taobao.com", 0.55},
		{"jd.com", 0.6},
		{"tmall.com", 0.55},

		// 低权威：内容农场/自媒体
		{"xiaohongshu", 0.35},
		{"weibo.com", 0.4},
		{"bilibili.com", 0.45},
		{"toutiao.com", 0.35},
		{"so.com", 0.4},
	}

	for _, kw := range keywords {
		if strings.Contains(host, kw.keyword) {
			return kw.score, true
		}
	}
	return 0, false
}

// exactDomainScores 精确域名白名单
var exactDomainScores = map[string]float64{
	// 政府
	"gov.cn":      1.0,
	"miit.gov.cn": 1.0,
	"moe.gov.cn":  1.0,
	"most.gov.cn": 1.0,

	// 教育
	"edu.cn": 0.95,

	// 百科
	"baike.baidu.com":  0.95,
	"zh.wikipedia.org": 0.9,
	"en.wikipedia.org": 0.9,
	"baike.sogou.com":  0.85,
	"baike.so.com":     0.8,

	// 知名技术站点
	"github.com":            0.75,
	"stackoverflow.com":     0.7,
	"docs.python.org":       0.85,
	"developer.mozilla.org": 0.85,
	"golang.org":            0.85,
	"nodejs.org":            0.85,
	"kubernetes.io":         0.85,
	"pytorch.org":           0.85,
	"tensorflow.org":        0.85,
	"react.dev":             0.8,
	"vuejs.org":             0.8,
	"angular.io":            0.8,
	"rust-lang.org":         0.85,
	"docs.docker.com":       0.8,
	"learn.microsoft.com":   0.85,

	// 知名媒体
	"people.com.cn":     0.85,
	"xinhuanet.com":     0.85,
	"cctv.com":          0.85,
	"chinadaily.com.cn": 0.8,
	"gmw.cn":            0.8,
	"youth.cn":          0.75,
	"chinanews.com.cn":  0.8,
	"thepaper.cn":       0.75,
	"guancha.cn":        0.75,

	// 学术
	"scholar.google.com": 0.85,
	"arxiv.org":          0.85,
	"cnki.net":           0.9,
	"wanfangdata.com.cn": 0.9,
	"sciencedirect.com":  0.9,
	"ieee.org":           0.9,
	"acm.org":            0.9,
	"springer.com":       0.9,
	"nature.com":         0.95,
	"science.org":        0.95,

	// 知名API/云服务文档
	"docs.aws.amazon.com":     0.8,
	"cloud.google.com":        0.8,
	"help.aliyun.com":         0.75,
	"cloud.tencent.com":       0.75,
	"support.huaweicloud.com": 0.75,
}

// ScoreResults 批量对搜索结果进行权威性评分
func ScoreResults(results []SearchResult) {
	for i := range results {
		results[i].AuthorityScore = ScoreDomainAuthority(results[i].URL, results[i].IsOfficial)
	}
}

// AuthorityLabel 返回权威性评分对应的文字标签
func AuthorityLabel(score float64) string {
	switch {
	case score >= 1.0:
		return "🏛️ 官方"
	case score >= 0.9:
		return "⭐ 高权威"
	case score >= 0.7:
		return "✅ 可信"
	case score >= 0.5:
		return "📄 一般"
	case score >= 0.3:
		return "⚠️ 低权威"
	default:
		return "❓ 未知"
	}
}

// SortResults 对搜索结果按相关性+权威性排序（所有模式共用）
// 规则：官方结果优先 → 标题完整匹配 → 摘要完整匹配 → 标题关键词匹配 → URL域名匹配
// query 用于相关性评分；传空字符串则只按权威性排序
func SortResults(results []SearchResult, query string) {
	if len(results) <= 1 {
		return
	}
	queryLower := strings.ToLower(query)
	type scored struct {
		result SearchResult
		score  int
	}
	scoredList := make([]scored, len(results))
	for i, r := range results {
		s := 0
		// 权威性评分（整数部分）
		s += int(r.AuthorityScore * 5) // 0~5 分

		// 官方标记：额外 +5 保证官方结果绝对优先
		if r.IsOfficial {
			s += 5
		}

		if queryLower != "" {
			titleLower := strings.ToLower(r.Title)
			snippetLower := strings.ToLower(r.Snippet)

			// 标题完整匹配查询
			if strings.Contains(titleLower, queryLower) {
				s += 8
			}
			// 摘要完整匹配查询
			if strings.Contains(snippetLower, queryLower) {
				s += 4
			}
			// URL 域名包含查询关键词中的词
			urlLower := strings.ToLower(r.URL)
			for _, w := range strings.Fields(queryLower) {
				if len(w) >= 2 && strings.Contains(urlLower, w) {
					s += 2
					break
				}
			}
		}
		scoredList[i] = scored{result: r, score: s}
	}

	// 按分数降序排序
	for i := 0; i < len(scoredList); i++ {
		for j := i + 1; j < len(scoredList); j++ {
			if scoredList[j].score > scoredList[i].score {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}
	for i, s := range scoredList {
		results[i] = s.result
	}
}
