package mediaobject

import (
	"net/url"
	"strings"
)

// HostOfURL 取 URL 主机名（解析失败返回空串）
func HostOfURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// IsVideoPlatformURL 判断是否为共享域名表中的视频平台地址。
//
// 发送侧用它决定 webpage.platform 是否声明为 web_video；
// 后端用它核对声明，双方共用同一份 VideoPlatformHosts。
func IsVideoPlatformURL(raw string) bool {
	host := HostOfURL(raw)
	if host == "" {
		return false
	}
	for _, domain := range VideoPlatformHosts {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
