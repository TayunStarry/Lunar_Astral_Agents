package resource

// url_norm.go URL 规范化与分类：本地路由、B 站页面地址、外部链接去跟踪参数

import (
	"net/url"
	"strings"
)

// normalizeLocalURL 把本地读取地址规整为相对路由
func normalizeLocalURL(raw string) string {
	s := strings.TrimSpace(raw)
	if cut := strings.IndexAny(s, "?#"); cut >= 0 {
		s = s[:cut]
	}
	rel := strings.TrimPrefix(strings.ReplaceAll(s, "\\", "/"), FileReadPrefix)
	rel = normalizeRel(rel)
	if rel == "" {
		return ""
	}
	return FileReadPrefix + rel
}

// isLoopbackHost 判断主机是否为本机
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0":
		return true
	}
	return false
}

// isBilibiliHost 判断域名是否属于 B 站
func isBilibiliHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, "bilibili.com") || strings.HasSuffix(host, "b23.tv")
}

// NormalizeURL 规范化 URL：本地路由去主机、B 站统一页面地址、外部链接去跟踪参数
func NormalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(s, FileReadPrefix) {
		return normalizeLocalURL(s)
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.Parse(s)
		if err != nil {
			return s
		}
		host := parsed.Hostname()
		if isLoopbackHost(host) && strings.HasPrefix(parsed.Path, FileReadPrefix) {
			return normalizeLocalURL(parsed.Path)
		}
		if bvid := BilibiliBvid(s); bvid != "" && isBilibiliHost(host) {
			canonical := "https://www.bilibili.com/video/" + bvid
			if page := parsed.Query().Get("p"); page != "" && page != "1" {
				canonical += "?p=" + page
			}
			return canonical
		}
		query := parsed.Query()
		for _, key := range urlTrackingParams {
			query.Del(key)
		}
		parsed.RawQuery = query.Encode()
		parsed.Fragment = ""
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		return parsed.String()
	}
	if bvid := BilibiliBvid(s); bvid != "" {
		return "https://www.bilibili.com/video/" + bvid
	}
	return s
}

// FileReadURL 由内容仓库相对路径生成本地目标 URL
func FileReadURL(rel string) string {
	rel = normalizeRel(rel)
	if rel == "" {
		return ""
	}
	return FileReadPrefix + rel
}

// RelFromURL 从本地目标 URL 反解内容仓库相对路径（非本地 URL 返回空串）
func RelFromURL(raw string) string {
	nURL := NormalizeURL(raw)
	if !IsLocalURL(nURL) {
		return ""
	}
	rel := strings.TrimPrefix(nURL, FileReadPrefix)
	return normalizeRel(rel)
}

// IsLocalURL 判断是否为本地资源路由
func IsLocalURL(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), FileReadPrefix)
}

// IsRemoteURL 判断是否为 http(s) 外部链接
func IsRemoteURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// URLKind 推断外部 URL 应落哪类句柄：B 站与视频直链 → V，动图直链 → G，其余 → U
func URLKind(raw, hint string) string {
	if hint != "" && IsHandleKind(hint) {
		return hint
	}
	nURL := NormalizeURL(raw)
	if IsLocalURL(nURL) {
		return KindOfExt(extOfURL(nURL))
	}
	if BilibiliBvid(nURL) != "" {
		return KindVideo
	}
	ext := extOfURL(nURL)
	switch KindOfExt(ext) {
	case KindVideo:
		return KindVideo
	case KindAudio:
		return KindAudio
	case KindImage:
		if MayBeAnimated(ext) {
			return KindAnimated
		}
		return KindImage
	}
	return KindURL
}

// extOfURL 取 URL 路径的扩展名（小写）
func extOfURL(raw string) string {
	path := raw
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	if index := strings.LastIndex(path, "."); index >= 0 {
		return strings.ToLower(path[index:])
	}
	return ""
}
