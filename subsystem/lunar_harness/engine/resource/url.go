package resource

// url.go 外部链接（网页 / 视频 / 动图）的 URL 主表登记
//
// 规则：
//   - 链接进入系统时只做登记与句柄化，不在接入阶段解析内容；
//   - 解析文本（parse_text）是该 URL 的唯一解读结果，写一次后 use_resource 直接返回；
//   - B 站立句柄指向完整页面 URL（不使用分片地址、不用 BV 号做主键）；
//   - 同一规范化 URL 复用同一句柄，数据库唯一索引兜底。

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// RememberWebPage 登记一条检索访问记录（不建句柄，仅写 URL 主表）
//
// WebSearch 的「近期已访问」与页面摘要共用同一张 URL 主表，
// 因此检索详情页不会额外占用句柄，但解析文本仍可被 use_resource 命中。
func RememberWebPage(rawURL, title, summary string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	norm := NormalizeURL(rawURL)
	if norm == "" {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	// B 站链接的解析文本只能由直驱印象 / 正式观看写入，检索摘要不得覆盖
	if BilibiliBvid(norm) != "" {
		_, _ = AdmitURL(URLAdmitInput{URL: norm, Title: title, Origin: "web-search"})
		return nil
	}
	// 已绑定句柄且已有解析文本：保持唯一绑定，不覆盖
	if existing, found := archiveDB.FindResourceByURL(norm); found &&
		existing.Handle != "" && existing.ParseState == ParseReady {
		return nil
	}
	res := &Resource{
		Kind:       KindURL,
		URL:        norm,
		Name:       firstNonEmpty(title, norm),
		Title:      firstNonEmpty(title, norm),
		ParseText:  summary,
		ParseState: ParseReady,
		Origin:     "web-search",
		Retain:     RetainSoft,
		State:      StateActive,
	}
	if err := archiveDB.upsertURLRecord(res); err != nil {
		return err
	}
	return archiveDB.setURLParseText(norm, summary)
}

// WebPageFresh 判断某 URL 的解析文本是否在 TTL 内（供「近期已访问」过滤）
func WebPageFresh(rawURL string, ttl time.Duration) bool {
	if EnsureReady() != nil || ttl <= 0 {
		return false
	}
	res, found := archiveDB.FindResourceByURL(rawURL)
	if !found || res.ParseState != ParseReady || strings.TrimSpace(res.ParseText) == "" {
		return false
	}
	var parsedAt int64
	_ = archiveDB.db.QueryRow(`SELECT parsed_at FROM url_master WHERE url = ?`, res.URL).Scan(&parsedAt)
	return time.Since(time.Unix(parsedAt, 0)) < ttl
}

// BilibiliBvid 从链接或文本中提取 BV 号（无则空串）
func BilibiliBvid(source string) string {
	return bvIDPattern.FindString(strings.TrimSpace(source))
}

// AdmitURL 登记链接资源并返回句柄；同一地址复用既有句柄
func AdmitURL(in URLAdmitInput) (AdmitResult, error) {
	if err := EnsureReady(); err != nil {
		return AdmitResult{}, err
	}
	rawURL := NormalizeURL(in.URL)
	if rawURL == "" {
		return AdmitResult{}, fmt.Errorf("链接地址不能为空")
	}
	origin := strings.TrimSpace(in.Origin)
	if origin == "" {
		origin = OriginChannel
	}
	title := strings.TrimSpace(in.Title)
	summary := strings.TrimSpace(in.Summary)

	if existing, found := archiveDB.FindResourceByURL(rawURL); found {
		archiveDB.TouchResource(existing.ID)
		newTitle := firstNonEmpty(title, existing.Title)
		newSummary := existing.ParseText
		if strings.TrimSpace(newSummary) == "" {
			newSummary = summary
		}
		if newTitle != existing.Title || newSummary != existing.ParseText {
			if err := archiveDB.UpdateResourceURLExtras(existing.ID, newTitle, newSummary); err != nil {
				logger.Warn("FileManager", "更新链接解析文本失败(%s): %v", rawURL, err)
			}
			existing.Title = newTitle
			existing.ParseText = newSummary
			if newSummary != "" {
				existing.ParseState = ParseReady
			}
		}
		if existing.Handle != "" {
			return admitResultOf(existing, true), nil
		}
	}
	kind := URLKind(rawURL, in.KindHint)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = firstNonEmpty(title, rawURL)
	}
	res := &Resource{
		Kind:       kind,
		URL:        rawURL,
		BlobPath:   RelFromURL(rawURL),
		Name:       name,
		MIME:       linkMIME(kind),
		Title:      firstNonEmpty(title, name),
		ParseText:  summary,
		ParseState: ParseNone,
		Origin:     origin,
		Retain:     DefaultRetain(origin),
		State:      StateActive,
		CreatedAt:  time.Now().Unix(),
		UsedAt:     time.Now().Unix(),
	}
	if summary != "" {
		res.ParseState = ParseReady
	}
	if err := archiveDB.InsertResource(res); err != nil {
		return AdmitResult{}, err
	}
	return admitResultOf(*res, false), nil
}

// linkMIME 链接资源的 MIME 标记
func linkMIME(kind string) string {
	switch kind {
	case KindVideo:
		return "text/uri-list"
	case KindAnimated:
		return "text/uri-list"
	case KindAudio:
		return "text/uri-list"
	}
	return URLMIME
}

// URLSummaryOf 读取已登记的解析文本（未登记或未解析返回空串）
func URLSummaryOf(rawURL string) string {
	return ParseTextOf(rawURL)
}

// ParseTextOf 读取某 URL 的唯一解析文本
func ParseTextOf(rawURL string) string {
	if EnsureReady() != nil {
		return ""
	}
	existing, found := archiveDB.FindResourceByURL(rawURL)
	if !found {
		return ""
	}
	if existing.ParseState != ParseReady {
		return ""
	}
	return strings.TrimSpace(existing.ParseText)
}

// SetParseText 写入某 URL 的解析文本（唯一绑定，失败时置 failed）
func SetParseText(rawURL, text string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	return archiveDB.setURLParseText(rawURL, text)
}

// MarkParseFailed 标记解析失败
func MarkParseFailed(rawURL string) {
	if err := EnsureReady(); err != nil {
		return
	}
	archiveDB.markURLParseFailed(rawURL)
}

// SetURLSummary 给已登记链接补写解析文本
func SetURLSummary(id int64, summary string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	res, found := archiveDB.GetResourceByID(id)
	if !found {
		return fmt.Errorf("链接资源不存在: %d", id)
	}
	return archiveDB.setURLParseText(res.URL, summary)
}

// LookupURLCache 按地址查解析文本与标题（未解析时只返回标题）
func LookupURLCache(rawURL string) (summary, title string) {
	if EnsureReady() != nil {
		return "", ""
	}
	res, found := archiveDB.FindResourceByURL(rawURL)
	if !found {
		return "", ""
	}
	if res.ParseState == ParseReady {
		summary = strings.TrimSpace(res.ParseText)
	}
	return summary, strings.TrimSpace(res.Title)
}

// URLLabel 链接的展示名：站点域名（解析失败时用中性词）
func URLLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "链接"
	}
	return parsed.Host
}

// UpgradeCDNScheme 把名单内的腾讯图床直链升级为 https；其它地址原样返回
func UpgradeCDNScheme(raw string) string {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "http://") {
		return trimmed
	}
	host := lower[len("http://"):]
	if cut := strings.IndexAny(host, "/?#"); cut >= 0 {
		host = host[:cut]
	}
	for _, candidate := range qqCDNHosts {
		if host == candidate {
			return "https://" + trimmed[len("http://"):]
		}
	}
	return trimmed
}

// ShortURLForLog 日志用的地址摘要：去掉查询串并截断长度
func ShortURLForLog(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if cut := strings.IndexAny(trimmed, "?#"); cut >= 0 {
		trimmed = trimmed[:cut]
	}
	if runes := []rune(trimmed); len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return trimmed
}

// formatURLContent 链接的取回文本：地址 + 站点 + 标题 + 解析文本
func formatURLContent(res Resource) string {
	var builder strings.Builder
	builder.WriteString("地址: ")
	builder.WriteString(strings.TrimSpace(res.URL))
	if IsLocalURL(res.URL) {
		builder.WriteString("\n本地: ")
		builder.WriteString(res.URL)
	} else {
		builder.WriteString("\n站点: ")
		builder.WriteString(URLLabel(res.URL))
	}
	if title := strings.TrimSpace(res.Title); title != "" && title != res.URL {
		builder.WriteString("\n标题: ")
		builder.WriteString(title)
	}
	if summary := strings.TrimSpace(res.ParseText); summary != "" {
		builder.WriteString("\n解析: ")
		builder.WriteString(summary)
	} else {
		builder.WriteString("\n解析: （这条链接还没有读到内容）")
	}
	return builder.String()
}
