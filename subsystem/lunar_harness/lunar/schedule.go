package lunar

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

var (
	cnDateRegex = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
	dailyRegex  = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	fullDateRe  = regexp.MustCompile(`^(\d{4})[年\-/.](\d{1,2})[月\-/.](\d{1,2})日?$`)
	yearMonthRe = regexp.MustCompile(`^(\d{4})[年\-/.](\d{1,2})月?$`)
	monthDayRe  = regexp.MustCompile(`^(\d{1,2})月(\d{1,2})日?$`)
	monthOnlyRe = regexp.MustCompile(`^(\d{1,2})月$`)
	yearOnlyRe  = regexp.MustCompile(`^(\d{4})年?$`)
	halfWidthRe = regexp.MustCompile(`[０-９]`)
	dashWidthRe = regexp.MustCompile(`[－—–]`)
)

var presetDailyTasks = []ScheduleItem{
	{ID: "daily_greeting_morning", Type: "daily", Time: "07:30", Content: "向用户发送早安问候，并提示可以查一下今天的天气"},
	{ID: "daily_greeting_evening", Type: "daily", Time: "22:00", Content: "向用户发送晚安问候，祝其好眠，并提示可以查看一下今天的新闻"},
	{ID: "daily_viewer_1100", Type: "daily", Time: "11:00", Content: "现在是上午 11 点。可以选择调用 viewer（观影者）刷一会儿视频（flow=watch_comment）；刷视频有每日次数限制，如果今天已经刷得够多，就继续做别的事。"},
	{ID: "daily_viewer_1700", Type: "daily", Time: "17:00", Content: "现在是下午 5 点。可以选择调用 viewer（观影者）刷一会儿视频（flow=watch_comment）；刷视频有每日次数限制，如果今天已经刷得够多，就继续做别的事。"},
	{ID: "daily_viewer_2000", Type: "daily", Time: "20:00", Content: "现在是晚上 8 点。可以选择调用 viewer（观影者）刷一会儿视频（flow=watch_comment）；刷视频有每日次数限制，如果今天已经刷得够多，就继续做别的事。"},
}

// initSchedules 初始化计划表
func initSchedules() {
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	items := loadSchedulesFromDisk()
	existing := map[string]bool{}
	for _, item := range items {
		existing[item.ID] = true
	}
	added := 0
	for _, preset := range presetDailyTasks {
		if existing[preset.ID] {
			continue
		}
		items = append(items, preset)
		added++
	}
	needsRewrite := added > 0
	for i := range items {
		if items[i].Type == "daily" {
			continue
		}
		if normalized := normalizeTime(items[i].Time); normalized != "" && normalized != items[i].Time {
			items[i].Time = normalized
			needsRewrite = true
		}
	}
	scheduleCache = items
	if needsRewrite {
		saveSchedulesToDisk(items)
	}
	logger.Info(lunarCoreModule, "[计划表] 初始化完成，共加载 %d 个计划项", len(items))
}

func loadSchedulesFromDisk() []ScheduleItem {
	file, _, _, err := persistence.ReadFile(scheduleFilePath)
	if err != nil {
		return nil
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var items []ScheduleItem
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	return items
}

func saveSchedulesToDisk(items []ScheduleItem) bool {
	text := safeJSON(items, "[]")
	if _, err := persistence.SaveFileEx(scheduleFilePath, persistence.SaveOptions{Overwrite: true}, strings.NewReader(text)); err != nil {
		logger.Warn(lunarCoreModule, "[计划表] 保存失败: %v", err)
		return false
	}
	return true
}

func generateScheduleID() string {
	runes := []rune("xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx")
	for i, ch := range runes {
		if ch != 'x' && ch != 'y' {
			continue
		}
		value := randomFloor(0, 15)
		if ch == 'y' {
			value = value&0x3 | 0x8
		}
		runes[i] = []rune(fmt.Sprintf("%x", value))[0]
	}
	return string(runes)
}

func formatISO(value time.Time) string {
	return value.Format("2006-01-02T15:04:05")
}

func formatDate(value time.Time) string {
	return value.Format("2006-01-02")
}

// normalizeTime 归一化时间格式
func normalizeTime(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	layouts := []string{
		"2006-01-02T15:04:05", "2006-01-02T15:04", time.RFC3339,
		"2006-01-02 15:04:05", "2006-01-02 15:04",
		"2006/1/2 15:04:05", "2006/1/2 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return formatISO(parsed)
		}
	}
	if match := cnDateRegex.FindStringSubmatch(trimmed); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		hour, _ := strconv.Atoi(match[4])
		minute, _ := strconv.Atoi(match[5])
		second := 0
		if match[6] != "" {
			second, _ = strconv.Atoi(match[6])
		}
		return formatISO(time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local))
	}
	return ""
}

func dailyTaskTime(item ScheduleItem, now time.Time) (time.Time, bool) {
	match := dailyRegex.FindStringSubmatch(strings.TrimSpace(item.Time))
	if match == nil {
		return time.Time{}, false
	}
	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local), true
}

// checkDueItems 检查到期计划项
func checkDueItems() []ScheduleItem {
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	if len(scheduleCache) == 0 {
		return nil
	}
	now := time.Now()
	today := formatDate(now)
	kept := make([]ScheduleItem, 0, len(scheduleCache))
	removed := make([]ScheduleItem, 0)
	type candidate struct {
		item    *ScheduleItem
		dueTime time.Time
	}
	candidates := make([]candidate, 0)
	for index := range scheduleCache {
		item := &scheduleCache[index]
		if item.Type == "daily" {
			due, ok := dailyTaskTime(*item, now)
			if ok && !now.Before(due) && item.CompletedDate != today {
				if now.Sub(due) > 30*time.Minute {
					item.CompletedDate = today
				} else {
					candidates = append(candidates, candidate{item: item, dueTime: due})
				}
			}
			kept = append(kept, *item)
			continue
		}
		due, err := time.ParseInLocation("2006-01-02T15:04:05", item.Time, time.Local)
		if err != nil {
			kept = append(kept, *item)
			continue
		}
		if now.Before(due) {
			kept = append(kept, *item)
			continue
		}
		if now.Sub(due) > 30*time.Minute {
			removed = append(removed, *item)
			continue
		}
		candidates = append(candidates, candidate{item: item, dueTime: due})
	}
	executed := make([]ScheduleItem, 0)
	if len(candidates) > 0 {
		best := 0
		for i := 1; i < len(candidates); i++ {
			a := absDuration(now.Sub(candidates[i].dueTime))
			b := absDuration(now.Sub(candidates[best].dueTime))
			if a < b {
				best = i
			}
		}
		chosen := candidates[best].item
		if chosen.Type == "daily" {
			chosen.CompletedDate = today
			for i := range kept {
				if kept[i].ID == chosen.ID {
					kept[i] = *chosen
				}
			}
		} else {
			removed = append(removed, *chosen)
		}
		executed = append(executed, *chosen)
	}
	if len(executed) > 0 || len(removed) > 0 {
		scheduleCache = kept
		saveSchedulesToDisk(scheduleCache)
	}
	return executed
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// flushDueScheduleItems 消费到期计划项并推进未读上下文
func flushDueScheduleItems() {
	due := checkDueItems()
	for _, item := range due {
		unreadMu.Lock()
		unreadContext = append(unreadContext, PostMessage{Role: RoleUser, Content: scheduleTriggerPrefix + " 预约时间已到，请执行以下计划：" + item.Content})
		unreadMu.Unlock()
	}
}

// ==== 查询 ====

type dateRange struct {
	Start time.Time
	End   time.Time
	Label string
}

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

func scheduleSortKey(item ScheduleItem, now time.Time) int64 {
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

func scheduleScore(item ScheduleItem, keywords []string) int {
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

// querySchedules 执行计划表查询
func querySchedules(cache []ScheduleItem, keywords []string, date, scope string) ToolResult {
	trimmedKeywords := make([]string, 0)
	for _, keyword := range keywords {
		if value := strings.TrimSpace(keyword); value != "" {
			trimmedKeywords = append(trimmedKeywords, value)
		}
	}
	rangeValue, errText := parseDateRange(date, scope)
	if errText != "" {
		return ToolResult{Text: "查询计划失败：" + errText}
	}
	now := time.Now()
	candidates := make([]ScheduleItem, 0, len(cache))
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
		item  ScheduleItem
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
			return ToolResult{Text: scopeText + "，没有任何计划项。"}
		}
		scopeText := "计划表"
		if rangeValue != nil {
			scopeText = rangeValue.Label
		}
		return ToolResult{Text: scopeText + fmt.Sprintf("共有 %d 个计划项，但没有命中关键词「%s」的计划。", len(candidates), strings.Join(trimmedKeywords, "、"))}
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
	return ToolResult{Text: builder.String()}
}
