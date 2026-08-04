package websearch

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ── URL 提取 ──

// urlRegex 匹配消息中的 http/https 链接
var urlRegex = regexp.MustCompile(`https?://[^\s]+`)

// bareURLRegex 匹配缺协议头的链接（如 example.com/path）
var bareURLRegex = regexp.MustCompile(`(?:^|[\s])[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z]{2,}(?:/[^\s]*)?`)

// extractURLs 从文本中提取所有 URL
func extractURLs(text string) []string {
	seen := make(map[string]bool)
	var urls []string

	// 匹配带协议的链接
	for _, u := range urlRegex.FindAllString(text, -1) {
		u = strings.TrimRight(u, ".,;:!?，。；：！？、】）)】\"'…")
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	// 匹配缺协议头的裸链接
	for _, m := range bareURLRegex.FindAllString(text, -1) {
		u := strings.TrimSpace(m)
		u = strings.TrimRight(u, ".,;:!?，。；：！？、】）)】\"'…")
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			continue // 已在上面处理过
		}
		// 过滤掉太短的假链接（如 "a.b"）
		if len(u) < 5 {
			continue
		}
		u = "https://" + u
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	return urls
}

// ── 链接分类 ──

// imageExtensions 图片文件后缀
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true, ".ico": true,
}

// downloadExtensions 下载文件后缀
var downloadExtensions = map[string]bool{
	".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".apk": true, ".exe": true, ".msi": true,
	".dmg": true, ".mp3": true, ".mp4": true, ".avi": true, ".mkv": true,
}

// classifyLink 根据URL后缀判断链接类型，无后缀时发HEAD请求检测Content-Type
func classifyLink(url string) LinkType {
	lower := strings.ToLower(url)

	// 去掉查询参数和锚点，提取路径部分
	path := lower
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if idx := strings.Index(path, "#"); idx >= 0 {
		path = path[:idx]
	}

	for ext := range imageExtensions {
		if strings.HasSuffix(path, ext) {
			return LinkImage
		}
	}
	for ext := range downloadExtensions {
		if strings.HasSuffix(path, ext) {
			return LinkDownload
		}
	}

	// 有明确网页后缀，直接判定为网页
	webExts := []string{".html", ".htm", ".php", ".asp", ".aspx", ".jsp"}
	for _, ext := range webExts {
		if strings.HasSuffix(path, ext) {
			return LinkWebpage
		}
	}

	// 无后缀，发 HEAD 请求检测
	contentType := detectContentType(url)
	if strings.HasPrefix(contentType, "image/") {
		return LinkImage
	}
	if strings.Contains(contentType, "application/") &&
		!strings.Contains(contentType, "html") &&
		!strings.Contains(contentType, "json") &&
		!strings.Contains(contentType, "xml") {
		return LinkDownload
	}

	return LinkWebpage
}

// detectContentType 通过 HEAD 请求检测 URL 的 Content-Type
func detectContentType(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", DefaultConfig.HTTP.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	return resp.Header.Get("Content-Type")
}

// ── 链接处理主入口 ──

// processAndReplaceLinks 提取消息中的链接，抓取内容并替换为编号格式
func (s *System) processAndReplaceLinks(query string) (string, map[string]string) {
	urls := extractURLs(query)
	if len(urls) == 0 {
		return query, nil
	}

	replacements := make(map[string]string, len(urls)) // url -> label
	linkMap := make(map[string]string, len(urls))      // "链接N" -> "url：摘要"
	totalChars := 0

	for i, u := range urls {
		linkType := classifyLink(u)

		var summary string
		switch linkType {
		case LinkImage:
			summary = s.processImageLink(u)
		case LinkDownload:
			if s.downloadFunc != nil {
				filePath, dlErr := s.downloadFunc(u, s.downloadGroupID)
				if dlErr != nil {
					summary = fmt.Sprintf("下载失败：%s", dlErr.Error())
				} else {
					summary = fmt.Sprintf("已下载：%s", filepath.Base(filePath))
				}
			} else {
				summary = "下载文件"
			}
		case LinkWebpage:
			summary = s.processWebpageLink(u)
		}

		// 预算控制
		summaryLen := len([]rune(summary))
		if totalChars+summaryLen > linkMaxTotalBudget {
			summary = truncateText(summary, linkMaxTotalBudget-totalChars)
			if summary == "" {
				break
			}
		}
		totalChars += summaryLen

		label := fmt.Sprintf("[链接%d]", i+1)
		replacements[u] = label
		linkMap[label] = fmt.Sprintf("%s：%s", u, summary)
	}

	replaced := replaceURLsInText(query, replacements)
	return replaced, linkMap
}

// replaceURLsInText 将文本中的 URL 替换为编号标签
func replaceURLsInText(text string, replacements map[string]string) string {
	result := text
	for url, label := range replacements {
		result = strings.Replace(result, url, label, 1)
	}
	return result
}

// ── 图片链接处理 ──

func (s *System) processImageLink(url string) string {
	if s.visionProvider == nil {
		return "图片链接（视觉模型未配置）"
	}

	desc, err := s.visionProvider.AnalyzeImage(url, "")
	if err != nil || desc == "" {
		return "图片链接（识别失败）"
	}

	return desc
}

// ── 网页链接处理 ──

func (s *System) processWebpageLink(url string) string {
	// 确保浏览器可用（懒加载），供 fetchContent 的 SPA 渲染使用
	s.EnsureBrowser()

	body, err := s.webpage.fetchContent(url, nil)
	if err != nil || len(body) == 0 {
		return "无法访问"
	}

	return truncateText(body, linkMaxRawContent)
}

// stripURLs 从文本中移除所有 URL，返回纯文字（用于搜索）
func stripURLs(text string, urls []string) string {
	result := text
	for _, u := range urls {
		result = strings.ReplaceAll(result, u, "")
	}
	return result
}
