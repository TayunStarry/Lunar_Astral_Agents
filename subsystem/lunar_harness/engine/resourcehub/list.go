package resourcehub

// list.go list 模式：列出符合标准的句柄数组。
//
// query 规则：
//   - 空的 query → 列出全部（或指定的类型）；
//   - 首项为合法类型码 D/I/G/A/V/U → 只列该类型；
//   - 其余项按文件名 / 纯文件夹名 / 路径过滤（多项取并集）。

import (
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// runList 列出资源目录并渲染句柄数组。
func runList(q Query) Result {
	kind := ""
	rest := q.Query
	if len(q.Query) > 0 {
		if code := strings.ToUpper(strings.TrimSpace(q.Query[0])); isKnownKind(code) {
			kind = code
			rest = q.Query[1:]
		}
	}
	keywords := make([]string, 0, len(rest))
	for _, raw := range rest {
		if keyword := normalizeListKeyword(raw); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = listDefaultLimit
	}
	if limit > listMaxLimit {
		limit = listMaxLimit
	}
	var (
		entries []resource.Resource
		err     error
	)
	if len(keywords) == 0 {
		entries, err = resource.List(kind, limit)
	} else {
		entries, err = resource.ListMatching(kind, keywords, limit)
	}
	if err != nil {
		return failure("资源目录查询失败：" + err.Error())
	}
	head := "当前可用资源"
	emptyHint := "资源目录为空：当前没有已收录的资源"
	if len(keywords) > 0 {
		head = "匹配的资源"
		emptyHint = "没有匹配「" + strings.Join(keywords, "、") + "」的资源；可用 fetch_resources（type=[\"list\"]）查看全部目录"
	}
	text, handles := renderResourceList(entries, head, emptyHint)
	return Result{Text: text, Handle: handles}
}

// normalizeListKeyword 归一化过滤词：去空白、反斜杠转正斜杠、去首尾路径分隔符
func normalizeListKeyword(raw string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"), "/")
}

// renderResourceList 目录统一格式化：自动清理失效句柄，输出句柄 / 类型 / 标题 / 摘要。
func renderResourceList(entries []resource.Resource, head, emptyHint string) (string, []string) {
	valid := make([]resource.Resource, 0, len(entries))
	purged := 0
	for _, entry := range entries {
		if entry.State == resource.StateLost {
			if resource.Delete(entry.Handle) == nil {
				purged++
			}
			continue
		}
		valid = append(valid, entry)
	}
	if len(valid) == 0 {
		note := ""
		if purged > 0 {
			note = "（已自动清理 " + strconv.Itoa(purged) + " 条源文件丢失的失效句柄）"
		}
		return emptyHint + note + "。", []string{}
	}
	stats, statsErr := resource.Stats()
	lines := make([]string, 0, len(valid))
	handles := make([]string, 0, len(valid))
	for _, entry := range valid {
		desc := ""
		if entry.Summary != "" {
			desc = "：" + previewRunes(strings.Join(strings.Fields(entry.Summary), " "), listSummaryRunes)
		}
		sectionNote := ""
		if entry.SectionCount > 0 {
			sectionNote = " " + strconv.Itoa(entry.SectionCount) + "节"
		}
		lines = append(lines, entry.Handle+" ["+kindLabel(entry.Kind)+"] "+entry.Title+sectionNote+desc)
		handles = append(handles, entry.Handle)
	}
	purgeNote := ""
	if purged > 0 {
		purgeNote = "；已自动清理 " + strconv.Itoa(purged) + " 条源文件丢失的失效句柄"
	}
	statsNote := ""
	if statsErr == nil {
		statsNote = "；目录总计 " + strconv.FormatInt(stats.Total, 10) + " 条 / " + formatBytes(stats.Bytes)
	}
	header := "（共 " + strconv.Itoa(len(valid)) + " 条，按收录时间倒序，越靠前越新" + purgeNote + statsNote + "）："
	tail := "\n需要内容时用 fetch_resources（type=[\"handle\"], query=[句柄]）取回，可一次传多个句柄。"
	return head + header + "\n" + strings.Join(lines, "\n") + tail, handles
}
