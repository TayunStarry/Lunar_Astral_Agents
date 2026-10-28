package resource

// url.go @U 链接资源 —— 网页 / 网络视频 / BV 号
//
// @U 与其它类型码的区别：不保存本体，只在资源目录登记「地址 + 标题 + 缓存摘要」。
// 摘要来源按优先级：
//   1. 调用方显式传入（哔哩观影的观影感受、网页检索摘要）；
//   2. media.db 的 video_summary（B 站视频按 URL / BV 号存过）；
//   3. web.db 的 web_ltp_page_cache（网页检索时存过标题与摘要）。
// 因此同一链接反复出现时复用同一句柄，并顺带把最新摘要补进目录。
//
// 展示与取回可以携带完整地址：模型需要地址时直接使用网址句柄的内容即可。

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/knowledge"
)

// BilibiliBvid 从链接或文本中提取 BV 号（无则空串）
func BilibiliBvid(source string) string {
	return bvIDPattern.FindString(strings.TrimSpace(source))
}

// AdmitURL 登记链接资源（@U），同一地址复用既有句柄
func AdmitURL(in URLAdmitInput) (AdmitResult, error) {
	if err := EnsureReady(); err != nil {
		return AdmitResult{}, err
	}
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return AdmitResult{}, fmt.Errorf("链接地址不能为空")
	}
	origin := strings.TrimSpace(in.Origin)
	if origin == "" {
		origin = OriginChannel
	}
	title := strings.TrimSpace(in.Title)
	summary := strings.TrimSpace(in.Summary)
	if title == "" || summary == "" {
		cachedSummary, cachedTitle := LookupURLCache(rawURL)
		if summary == "" {
			summary = cachedSummary
		}
		if title == "" {
			title = cachedTitle
		}
	}
	if existing, found := archiveDB.FindResourceByURL(rawURL); found {
		archiveDB.TouchResource(existing.ID)
		if title != existing.Title || summary != existing.Summary {
			if err := archiveDB.UpdateResourceURLExtras(existing.ID, firstNonEmpty(title, existing.Title), firstNonEmpty(summary, existing.Summary)); err != nil {
				logger.Warn("FileManager", "更新链接摘要失败(%s): %v", rawURL, err)
			}
			existing.Title = firstNonEmpty(title, existing.Title)
			existing.Summary = firstNonEmpty(summary, existing.Summary)
		}
		return admitResultOf(existing, true), nil
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		// 不再隐藏完整地址：没有标题时直接用地址作为展示名。
		name = firstNonEmpty(title, rawURL)
	}
	res := &Resource{
		Kind:      KindURL,
		Name:      name,
		MIME:      URLMIME,
		URL:       rawURL,
		Title:     firstNonEmpty(title, name),
		Summary:   summary,
		Origin:    origin,
		Retain:    DefaultRetain(origin),
		State:     StateActive,
		CreatedAt: time.Now().Unix(),
		UsedAt:    time.Now().Unix(),
	}
	if err := archiveDB.InsertResource(res); err != nil {
		return AdmitResult{}, err
	}
	return admitResultOf(*res, false), nil
}

// URLSummaryOf 读取资源目录里已登记的链接摘要（未登记返回空串）
func URLSummaryOf(rawURL string) string {
	if err := EnsureReady(); err != nil {
		return ""
	}
	existing, found := archiveDB.FindResourceByURL(strings.TrimSpace(rawURL))
	if !found {
		return ""
	}
	return strings.TrimSpace(existing.Summary)
}

// SetURLSummary 给 @U 资源补写摘要（缓存摘要后到时回填）
func SetURLSummary(id int64, summary string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	res, found := archiveDB.GetResourceByID(id)
	if !found || res.Kind != KindURL {
		return fmt.Errorf("链接资源不存在: %d", id)
	}
	return archiveDB.UpdateResourceURLExtras(id, res.Title, summary)
}

// LookupURLCache 按地址查缓存的标题与摘要（B 站视频摘要优先，其次网页检索缓存）
func LookupURLCache(rawURL string) (summary, title string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ""
	}
	keys := []string{rawURL}
	if bvid := BilibiliBvid(rawURL); bvid != "" {
		keys = append(keys, bvid)
	}
	for _, key := range keys {
		if value := knowledge.Value("video_summary", key); strings.TrimSpace(value) != "" {
			summary = strings.TrimSpace(value)
			break
		}
	}
	webTitle, webSummary := knowledge.WebPageSummary(rawURL)
	if summary == "" {
		summary = webSummary
	}
	return summary, webTitle
}

// URLLabel 链接的展示名：站点域名（解析失败时用中性词）
func URLLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "链接"
	}
	return parsed.Host
}

// formatURLContent @U 的取回文本：地址 + 站点 + 标题 + 缓存摘要
func formatURLContent(res Resource) string {
	var builder strings.Builder
	builder.WriteString("地址: ")
	builder.WriteString(strings.TrimSpace(res.URL))
	builder.WriteString("\n站点: ")
	builder.WriteString(URLLabel(res.URL))
	if title := strings.TrimSpace(res.Title); title != "" && title != res.URL {
		builder.WriteString("\n标题: ")
		builder.WriteString(title)
	}
	if summary := strings.TrimSpace(res.Summary); summary != "" {
		builder.WriteString("\n摘要: ")
		builder.WriteString(summary)
	} else {
		builder.WriteString("\n摘要: （这条链接还没有读到内容）")
	}
	return builder.String()
}
