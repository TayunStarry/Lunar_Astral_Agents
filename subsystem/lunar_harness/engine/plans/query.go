package plans

// query.go 计划表查询：按日期范围与关键词过滤，返回可直接交给模型的结果文本。

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Query 执行计划表查询。
func Query(keywords []string, date, scope string) (string, error) {
	trimmedKeywords := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		if value := strings.TrimSpace(keyword); value != "" {
			trimmedKeywords = append(trimmedKeywords, value)
		}
	}
	rangeValue, errText := parseDateRange(date, scope)
	if errText != "" {
		return "", fmt.Errorf("%s", errText)
	}
	now := time.Now()
	cache := List()
	candidates := make([]Item, 0, len(cache))
	for _, item := range cache {
		if rangeValue == nil {
			candidates = append(candidates, item)
			continue
		}
		if item.Type == "daily" {
			candidates = append(candidates, item)
			continue
		}
		parsed, err := time.ParseInLocation("2006-01-02T15:04:05", item.Time, time.Local)
		if err == nil && !parsed.Before(rangeValue.Start) && !parsed.After(rangeValue.End) {
			candidates = append(candidates, item)
		}
	}
	type matchedItem struct {
		item  Item
		score int
	}
	matched := make([]matchedItem, 0)
	for _, item := range candidates {
		score := 0
		if len(trimmedKeywords) > 0 {
			score = scheduleScore(item, trimmedKeywords)
			if score == 0 {
				continue
			}
		}
		matched = append(matched, matchedItem{item: item, score: score})
	}
	for i := 1; i < len(matched); i++ {
		for j := i; j > 0; j-- {
			left, right := matched[j-1], matched[j]
			if right.score > left.score || (right.score == left.score && scheduleSortKey(right.item, now) < scheduleSortKey(left.item, now)) {
				matched[j-1], matched[j] = right, left
			}
		}
	}
	if len(matched) == 0 {
		if len(candidates) == 0 {
			scopeText := "当前计划表为空"
			if rangeValue != nil {
				scopeText = rangeValue.Label + "没有安排任何计划"
			}
			return scopeText + "，没有任何计划项。", nil
		}
		scopeText := "计划表"
		if rangeValue != nil {
			scopeText = rangeValue.Label
		}
		return scopeText + fmt.Sprintf("共有 %d 个计划项，但没有命中关键词「%s」的计划。", len(candidates), strings.Join(trimmedKeywords, "、")), nil
	}
	var builder strings.Builder
	if rangeValue != nil {
		builder.WriteString(fmt.Sprintf("%s共有 %d 个计划项", rangeValue.Label, len(candidates)))
	} else {
		builder.WriteString(fmt.Sprintf("当前计划表共有 %d 个计划项", len(candidates)))
	}
	if len(trimmedKeywords) > 0 {
		builder.WriteString(fmt.Sprintf("，命中「%s」的有 %d 条（按相关度排序）", strings.Join(trimmedKeywords, "、"), len(matched)))
	} else {
		builder.WriteString("（按时间排序）")
	}
	for i, entry := range matched {
		displayTime := entry.item.Time
		if entry.item.Type == "daily" {
			displayTime = "每日 " + entry.item.Time
		}
		builder.WriteString(fmt.Sprintf("\n[计划项%d] ID:%s | 时间:%s | 内容:%s", i+1, entry.item.ID, displayTime, entry.item.Content))
	}
	return builder.String(), nil
}

// parseDateRange 解析查询日期；返回 nil 表示全部范围。
func parseDateRange(raw, scope string) (*dateRange, string) {
	now := time.Now()
	normalized := toHalfWidth(strings.TrimSpace(raw))
	relative := map[string]int{"今天": 0, "今日": 0, "明天": 1, "明日": 1, "后天": 2, "昨天": -1, "昨日": -1}
	if offset, ok := relative[normalized]; ok {
		base := now.AddDate(0, 0, offset)
		suffix := "（今天）"
		if offset != 0 {
			suffix = "（" + normalized + "）"
		}
		rangeValue := dayRange(base, suffix)
		return &rangeValue, ""
	}
	if normalized == "" {
		switch scope {
		case "month":
			rangeValue := monthRange(now.Year(), int(now.Month()))
			return &rangeValue, ""
		case "year":
			rangeValue := yearRange(now.Year())
			return &rangeValue, ""
		case "all":
			return nil, ""
		default:
			rangeValue := dayRange(now, "（今天）")
			return &rangeValue, ""
		}
	}
	if match := fullDateRe.FindStringSubmatch(normalized); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		rangeValue := dayRange(time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local), "")
		return &rangeValue, ""
	}
	if match := yearMonthRe.FindStringSubmatch(normalized); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		rangeValue := monthRange(year, month)
		return &rangeValue, ""
	}
	if match := monthDayRe.FindStringSubmatch(normalized); match != nil {
		month, _ := strconv.Atoi(match[1])
		day, _ := strconv.Atoi(match[2])
		rangeValue := dayRange(time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, time.Local), "")
		return &rangeValue, ""
	}
	if match := monthOnlyRe.FindStringSubmatch(normalized); match != nil {
		month, _ := strconv.Atoi(match[1])
		rangeValue := monthRange(now.Year(), month)
		return &rangeValue, ""
	}
	if match := yearOnlyRe.FindStringSubmatch(normalized); match != nil {
		year, _ := strconv.Atoi(match[1])
		rangeValue := yearRange(year)
		return &rangeValue, ""
	}
	if parsed, err := time.ParseInLocation("2006-01-02", normalized, time.Local); err == nil {
		rangeValue := dayRange(parsed, "")
		return &rangeValue, ""
	}
	return nil, fmt.Sprintf("无法识别日期 %q，请使用如 \"2026-06-14\"、\"2026-06\"、\"2026\" 或 \"今天\"", raw)
}

// scheduleSortKey 查询排序键。
func scheduleSortKey(item Item, now time.Time) int64 {
	if item.Type == "daily" {
		if value, ok := dailyTaskTime(item, now); ok {
			return value.UnixMilli()
		}
		return int64(^uint64(0) >> 1)
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", item.Time, time.Local)
	if err != nil {
		return int64(^uint64(0) >> 1)
	}
	return parsed.UnixMilli()
}

// scheduleScore 关键词相关度评分。
func scheduleScore(item Item, keywords []string) int {
	content := strings.ToLower(item.Content)
	score := 0
	for _, keyword := range keywords {
		lower := strings.ToLower(keyword)
		if lower == "" {
			continue
		}
		if strings.Contains(content, lower) {
			score += 2
		} else if strings.Contains(item.Time, keyword) {
			score++
		}
	}
	return score
}

// toHalfWidth 全角数字与连字符转半角。
func toHalfWidth(raw string) string {
	raw = halfWidthRe.ReplaceAllStringFunc(raw, func(ch string) string {
		code := []rune(ch)[0]
		return string(rune(code - 0xFEE0))
	})
	return dashWidthRe.ReplaceAllString(raw, "-")
}

func dayRange(base time.Time, suffix string) dateRange {
	return dateRange{
		Start: time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local),
		End:   time.Date(base.Year(), base.Month(), base.Day(), 23, 59, 59, 999999999, time.Local),
		Label: formatDate(base) + suffix,
	}
}

func monthRange(year, month int) dateRange {
	return dateRange{
		Start: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(year, time.Month(month)+1, 0, 23, 59, 59, 999999999, time.Local),
		Label: fmt.Sprintf("%d年%d月", year, month),
	}
}

func yearRange(year int) dateRange {
	return dateRange{
		Start: time.Date(year, 1, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(year, 12, 31, 23, 59, 59, 999999999, time.Local),
		Label: fmt.Sprintf("%d年", year),
	}
}
