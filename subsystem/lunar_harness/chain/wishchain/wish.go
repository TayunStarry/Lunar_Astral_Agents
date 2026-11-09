package wishchain

// wish.go 愿望的增删改查、优先级、到期通知与结算。
//
// 容量规则：待实现列表最多 16 条；优先级取 0-15 的整数，0 为最高优先级。
// 每次写入都会原子落盘到统一持久化文件。

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// AutoPriority 表示由系统自动分配最低可用优先级。
const AutoPriority = -1

// List 返回按优先级从高到低排序的待实现愿望副本。
func List() []Wish {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	return pendingLocked()
}

// All 返回全部愿望（待实现在前，已完成在后）。
func All() []Wish {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	pending := pendingLocked()
	out := append([]Wish(nil), pending...)
	completed := make([]Wish, 0)
	for _, item := range state.Wishes {
		if item.Status == StatusCompleted {
			completed = append(completed, item)
		}
	}
	sort.SliceStable(completed, func(i, j int) bool { return completed[i].CompletedAt > completed[j].CompletedAt })
	return append(out, completed...)
}

// CountPending 返回待实现愿望数量。
func CountPending() int {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	return countPendingLocked()
}

// Add 新增一条愿望；priority 传 AutoPriority 时自动取最低可用优先级。
func Add(content, due string, priority int) (Wish, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Wish{}, fmt.Errorf("愿望内容不能为空")
	}
	normalizedDue, err := normalizeDue(due)
	if err != nil {
		return Wish{}, err
	}

	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	if countPendingLocked() >= MaxPending {
		return Wish{}, ErrCapacity
	}
	if priority == AutoPriority {
		priority = nextPriorityLocked()
	}
	if err := validatePriority(priority); err != nil {
		return Wish{}, err
	}
	now := time.Now().Unix()
	item := Wish{
		ID:        generateID(),
		Content:   content,
		Due:       normalizedDue,
		Priority:  priority,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state.Wishes = append(state.Wishes, item)
	saveLocked()
	logger.Info(Module, "[愿望] 新增 %s：P%d %s（%s）", item.ID, item.Priority, item.Content, item.Due)
	return item, nil
}

// Update 改写愿望：内容 / 期限 / 优先级至少提供一个；priority 传 AutoPriority 表示不改。
func Update(id, content, due string, priority int) (Wish, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Wish{}, fmt.Errorf("愿望 ID 不能为空")
	}
	content = strings.TrimSpace(content)
	due = strings.TrimSpace(due)
	if content == "" && due == "" && priority == AutoPriority {
		return Wish{}, fmt.Errorf("至少需要提供内容、期限或优先级")
	}
	normalizedDue, err := normalizeDue(due)
	if err != nil {
		return Wish{}, err
	}
	if priority != AutoPriority {
		if err := validatePriority(priority); err != nil {
			return Wish{}, err
		}
	}

	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := findLocked(id)
	if index < 0 {
		return Wish{}, fmt.Errorf("未找到愿望 %s", id)
	}
	item := &state.Wishes[index]
	if item.Status == StatusCompleted {
		return Wish{}, fmt.Errorf("愿望 %s 已完成，不能再改写", id)
	}
	if content != "" {
		item.Content = content
	}
	if normalizedDue != "" {
		if oldDue, ok := parseWishDue(item.Due); ok {
			if newDue, ok := parseWishDue(normalizedDue); ok && newDue.After(oldDue) {
				item.PostponedCount++
				// 延期后重新进入临近 / 逾期判断。
				item.NearSettled = false
				item.Notified = false
			}
		}
		item.Due = normalizedDue
		// 期限改到未来后重新允许一次到期提醒。
		if newDue, ok := parseWishDue(normalizedDue); ok && newDue.After(time.Now()) {
			item.Notified = false
			item.NearSettled = false
		}
	}
	if priority != AutoPriority {
		item.Priority = priority
	}
	item.UpdatedAt = time.Now().Unix()
	saveLocked()
	logger.Info(Module, "[愿望] 改写 %s：P%d %s（%s）", item.ID, item.Priority, item.Content, item.Due)
	return *item, nil
}

// SetPriority 只调整优先级。
func SetPriority(id string, priority int) (Wish, error) {
	if err := validatePriority(priority); err != nil {
		return Wish{}, err
	}
	id = strings.TrimSpace(id)
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := findLocked(id)
	if index < 0 {
		return Wish{}, fmt.Errorf("未找到愿望 %s", id)
	}
	item := &state.Wishes[index]
	if item.Status == StatusCompleted {
		return Wish{}, fmt.Errorf("愿望 %s 已完成，不能再调整优先级", id)
	}
	item.Priority = priority
	item.UpdatedAt = time.Now().Unix()
	saveLocked()
	logger.Info(Module, "[愿望] 调整优先级 %s：P%d %s", item.ID, item.Priority, item.Content)
	return *item, nil
}

// Complete 标记愿望完成。
func Complete(id string) (Wish, error) {
	id = strings.TrimSpace(id)
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := findLocked(id)
	if index < 0 {
		return Wish{}, fmt.Errorf("未找到愿望 %s", id)
	}
	item := &state.Wishes[index]
	if item.Status == StatusCompleted {
		return *item, nil
	}
	now := time.Now().Unix()
	item.Status = StatusCompleted
	item.CompletedAt = now
	item.UpdatedAt = now
	item.CompletedSettled = false
	saveLocked()
	logger.Info(Module, "[愿望] 完成 %s：%s", item.ID, item.Content)
	return *item, nil
}

// Delete 删除愿望。
func Delete(id string) (Wish, error) {
	id = strings.TrimSpace(id)
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := findLocked(id)
	if index < 0 {
		return Wish{}, fmt.Errorf("未找到愿望 %s", id)
	}
	deleted := state.Wishes[index]
	state.Wishes = append(state.Wishes[:index], state.Wishes[index+1:]...)
	saveLocked()
	logger.Info(Module, "[愿望] 删除 %s：%s", deleted.ID, deleted.Content)
	return deleted, nil
}

// Get 读取单条愿望。
func Get(id string) (Wish, bool) {
	id = strings.TrimSpace(id)
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := findLocked(id)
	if index < 0 {
		return Wish{}, false
	}
	return state.Wishes[index], true
}

// Query 按关键词筛选愿望并返回可直接交给模型的结果文本。
func Query(keywords []string) string {
	trimmed := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		if value := strings.TrimSpace(keyword); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	all := All()
	if len(all) == 0 {
		return "当前没有待实现的愿望。"
	}
	matched := make([]Wish, 0, len(all))
	for _, item := range all {
		if matchesKeywords(item, trimmed) {
			matched = append(matched, item)
		}
	}
	if len(matched) == 0 {
		return fmt.Sprintf("当前共有 %d 条愿望，但没有命中关键词「%s」的愿望。",
			len(all), strings.Join(trimmed, "、"))
	}
	var builder strings.Builder
	if len(trimmed) > 0 {
		builder.WriteString(fmt.Sprintf("当前共有 %d 条愿望，命中「%s」的有 %d 条（按优先级排序）",
			len(all), strings.Join(trimmed, "、"), len(matched)))
	} else {
		builder.WriteString(fmt.Sprintf("当前共有 %d 条愿望（按优先级排序）", len(matched)))
	}
	for _, item := range matched {
		builder.WriteString("\n")
		builder.WriteString(formatWishLine(item))
	}
	return builder.String()
}

// FormatList 返回供运行时消息展示的完整愿望清单（按优先级从高到低）。
func FormatList() string {
	items := List()
	if len(items) == 0 {
		return "【愿望清单】当前没有待实现的愿望。"
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("【愿望清单】（优先级 0 最高，共 %d 条）", len(items)))
	for _, item := range items {
		builder.WriteString("\n")
		builder.WriteString(formatWishLine(item))
	}
	return builder.String()
}

// Upcoming 返回未来 window 内即将到期的愿望摘要；没有则返回空串。
func Upcoming(now time.Time, window time.Duration) string {
	mu.Lock()
	ensureLoadedLocked()
	snapshot := pendingLocked()
	mu.Unlock()
	parts := make([]string, 0)
	for _, item := range snapshot {
		due, ok := parseWishDue(item.Due)
		if !ok || !due.After(now) || due.After(now.Add(window)) {
			continue
		}
		parts = append(parts, due.Format("15:04")+" "+textutil.TruncateRunes(item.Content, 128))
	}
	return strings.Join(parts, "；")
}

// ConsumeDue 取出发送过到期通知的愿望；宽限期内不算逾期。
func ConsumeDue(now time.Time) []DueNotice {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	notices := make([]DueNotice, 0)
	for index := range state.Wishes {
		item := &state.Wishes[index]
		if item.Status != StatusPending || item.Notified {
			continue
		}
		due, ok := parseWishDue(item.Due)
		if !ok || now.Before(due) {
			continue
		}
		item.Notified = true
		notices = append(notices, DueNotice{
			Wish:    *item,
			Overdue: now.Sub(due) > GraceDuration,
			At:      now,
		})
	}
	if len(notices) > 0 {
		saveLocked()
		logger.Info(Module, "[愿望] %d 条愿望到期，已生成提醒", len(notices))
	}
	return notices
}

// Settle 执行一轮愿望结算：把延期 / 临近 / 完成 / 逾期映射为心情偏移。
//
// 已完成的愿望只在「完成 + 心情结算」都完成后才被清除，避免重复计算。
func Settle(now time.Time) Settlement {
	// 心情锚点未就绪时先尝试初始化；失败则本轮不施加联动，标记留在下一轮重试。
	mu.Lock()
	ready := initialized
	mu.Unlock()
	if !ready {
		_ = Init(context.Background())
	}
	type candidate struct {
		wishID string
		reason string
		label  string
	}
	mu.Lock()
	ensureLoadedLocked()
	candidates := make([]candidate, 0)
	for index := range state.Wishes {
		item := &state.Wishes[index]
		if item.Status == StatusCompleted {
			if !item.CompletedSettled {
				candidates = append(candidates, candidate{wishID: item.ID, reason: "completed", label: "满足"})
			}
			continue
		}
		if item.PostponedCount > item.PostponedSettled {
			candidates = append(candidates, candidate{wishID: item.ID, reason: "postponed", label: "遗憾"})
		}
		due, ok := parseWishDue(item.Due)
		if !ok {
			continue
		}
		if !item.NearSettled && !item.Notified && !now.Before(due.Add(-NearWindow)) && now.Before(due) {
			candidates = append(candidates, candidate{wishID: item.ID, reason: "near", label: "开心"})
		}
		// 逾期不销毁、不设一次性标记：每一次结算都重新施加失望，
		// 直到愿望被完成或删除。
		if now.After(due.Add(GraceDuration)) {
			candidates = append(candidates, candidate{wishID: item.ID, reason: "overdue", label: "失望"})
		}
	}
	mu.Unlock()
	if len(candidates) == 0 {
		return Settlement{}
	}

	applied := make([]MoodChange, 0, len(candidates))
	settled := make(map[string]map[string]bool)
	for _, item := range candidates {
		if !NudgeMood(item.label, moodStrength(item.reason)) {
			continue
		}
		applied = append(applied, MoodChange{WishID: item.wishID, Reason: item.reason, Label: item.label})
		if settled[item.wishID] == nil {
			settled[item.wishID] = map[string]bool{}
		}
		settled[item.wishID][item.reason] = true
	}

	mu.Lock()
	defer mu.Unlock()
	removed := make([]string, 0)
	kept := make([]Wish, 0, len(state.Wishes))
	for _, item := range state.Wishes {
		if reasons := settled[item.ID]; len(reasons) > 0 {
			if reasons["postponed"] {
				item.PostponedSettled = item.PostponedCount
			}
			if reasons["near"] {
				item.NearSettled = true
			}
			if reasons["completed"] {
				item.CompletedSettled = true
			}
		}
		if item.Status == StatusCompleted && item.CompletedSettled {
			removed = append(removed, item.ID)
			continue
		}
		kept = append(kept, item)
	}
	state.Wishes = kept
	saveLocked()
	if len(applied) > 0 {
		logger.Info(Module, "[心愿] 本轮结算 %d 项愿望联动，清除已完成 %d 条", len(applied), len(removed))
	}
	return Settlement{Applied: applied, Removed: removed}
}

// moodStrength 不同愿望事件对心情的影响强度。
func moodStrength(reason string) float64 {
	switch reason {
	case "completed":
		return 0.22
	case "overdue":
		return 0.20
	case "postponed":
		return 0.18
	case "near":
		return 0.12
	default:
		return 0.10
	}
}

// pendingLocked 返回按优先级排序的待实现愿望；调用方需持有锁。
func pendingLocked() []Wish {
	out := make([]Wish, 0, len(state.Wishes))
	for _, item := range state.Wishes {
		if item.Status == StatusPending {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		dueI, okI := parseWishDue(out[i].Due)
		dueJ, okJ := parseWishDue(out[j].Due)
		switch {
		case okI && okJ:
			if !dueI.Equal(dueJ) {
				return dueI.Before(dueJ)
			}
		case okI != okJ:
			return okI
		}
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

// countPendingLocked 待实现数量；调用方需持有锁。
func countPendingLocked() int {
	count := 0
	for _, item := range state.Wishes {
		if item.Status == StatusPending {
			count++
		}
	}
	return count
}

// nextPriorityLocked 取 0-15 中最小可用优先级；调用方需持有锁。
func nextPriorityLocked() int {
	used := make(map[int]bool, MaxPending)
	for _, item := range state.Wishes {
		if item.Status == StatusPending {
			used[item.Priority] = true
		}
	}
	for priority := MinPriority; priority <= MaxPriority; priority++ {
		if !used[priority] {
			return priority
		}
	}
	return MaxPriority
}

// validatePriority 校验优先级范围。
func validatePriority(priority int) error {
	if priority < MinPriority || priority > MaxPriority {
		return fmt.Errorf("优先级必须是 %d-%d 的整数（0 最高）", MinPriority, MaxPriority)
	}
	return nil
}

// findLocked 查找愿望下标；未找到返回 -1。
func findLocked(id string) int {
	for index := range state.Wishes {
		if state.Wishes[index].ID == id {
			return index
		}
	}
	return -1
}

// matchesKeywords 关键词为空时全部命中；否则内容包含任一关键词即命中。
func matchesKeywords(item Wish, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	content := strings.ToLower(item.Content)
	for _, keyword := range keywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// formatWishLine 单条愿望的展示文本。
func formatWishLine(item Wish) string {
	due := "无期限"
	if parsed, ok := parseWishDue(item.Due); ok {
		due = parsed.Format("2006-01-02 15:04")
	}
	if isOverdue(item, time.Now()) {
		due = "已逾期"
	}
	return fmt.Sprintf("P%d｜%s｜%s｜ID:%s", item.Priority, due, item.Content, item.ID)
}

// normalizeDue 归一化期限；空串表示没有期限。
func normalizeDue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	normalized := NormalizeTime(raw)
	if normalized == "" {
		return "", fmt.Errorf("无法解析时间格式 %q", raw)
	}
	return normalized, nil
}

// parseWishDue 解析愿望期限。
func parseWishDue(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// NormalizeTime 归一化时间格式；无法解析返回空串。
func NormalizeTime(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	for _, layout := range timeLayouts {
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

// formatISO 统一时间文本。
func formatISO(value time.Time) string { return value.Format("2006-01-02T15:04:05") }

// generateID 生成 UUID v4 形式的主键。
func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
