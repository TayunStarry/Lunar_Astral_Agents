package plans

// store.go 计划表的加载、保存与增删改。

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// Init 载入计划表并补齐默认每日计划。
func Init() {
	mu.Lock()
	defer mu.Unlock()
	loaded := loadLocked()
	existing := map[string]bool{}
	for _, item := range loaded {
		existing[item.ID] = true
	}
	added := 0
	for _, preset := range presetDailyTasks {
		if existing[preset.ID] {
			continue
		}
		loaded = append(loaded, preset)
		added++
	}
	needsRewrite := added > 0
	for i := range loaded {
		if loaded[i].Type == "daily" {
			continue
		}
		if normalized := NormalizeTime(loaded[i].Time); normalized != "" && normalized != loaded[i].Time {
			loaded[i].Time = normalized
			needsRewrite = true
		}
	}
	items = loaded
	if needsRewrite {
		saveLocked()
	}
	logger.Info(Module, "[计划表] 初始化完成，共加载 %d 个计划项", len(items))
}

// List 返回计划表副本。
func List() []Item {
	mu.Lock()
	defer mu.Unlock()
	return append([]Item(nil), items...)
}

// Create 新增计划项。
func Create(at, content string) (Item, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Item{}, fmt.Errorf("计划内容不能为空")
	}
	normalized := NormalizeTime(at)
	if normalized == "" {
		return Item{}, fmt.Errorf("无法解析时间格式 %q", at)
	}
	mu.Lock()
	defer mu.Unlock()
	item := Item{ID: generateID(), Time: normalized, Content: content}
	items = append(items, item)
	if err := saveLocked(); err != nil {
		items = items[:len(items)-1]
		return Item{}, err
	}
	logger.Info(Module, "[计划表] 新增计划 %s：%s %s", item.ID, item.Time, item.Content)
	return item, nil
}

// Update 改写计划项：at / content 至少提供一个。
func Update(id, at, content string) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, fmt.Errorf("计划项 ID 不能为空")
	}
	at = strings.TrimSpace(at)
	content = strings.TrimSpace(content)
	if at == "" && content == "" {
		return Item{}, fmt.Errorf("至少需要提供时间或内容")
	}
	mu.Lock()
	defer mu.Unlock()
	index := findLocked(id)
	if index < 0 {
		return Item{}, fmt.Errorf("未找到计划项 %s", id)
	}
	snapshot := items[index]
	if at != "" {
		normalized := NormalizeTime(at)
		if normalized == "" {
			return Item{}, fmt.Errorf("无法解析时间格式 %q", at)
		}
		items[index].Time = normalized
	}
	if content != "" {
		items[index].Content = content
	}
	if err := saveLocked(); err != nil {
		items[index] = snapshot
		return Item{}, err
	}
	logger.Info(Module, "[计划表] 改写计划 %s：%s %s", id, items[index].Time, items[index].Content)
	return items[index], nil
}

// Delete 删除计划项。
func Delete(id string) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, fmt.Errorf("计划项 ID 不能为空")
	}
	mu.Lock()
	defer mu.Unlock()
	index := findLocked(id)
	if index < 0 {
		return Item{}, fmt.Errorf("未找到计划项 %s", id)
	}
	deleted := items[index]
	items = append(items[:index], items[index+1:]...)
	if err := saveLocked(); err != nil {
		items = append(items[:index], append([]Item{deleted}, items[index:]...)...)
		return Item{}, err
	}
	logger.Info(Module, "[计划表] 删除计划 %s：%s", id, deleted.Content)
	return deleted, nil
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

// loadLocked 从磁盘读取计划表；调用方需持有锁。
func loadLocked() []Item {
	raw, ok := persistence.ReadFileBytes(storageFile)
	if !ok || len(raw) == 0 {
		return nil
	}
	var saved []Item
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[计划表] 存储文件损坏，按空计划表继续: %v", err)
		return nil
	}
	return saved
}

// saveLocked 原子写入计划表；调用方需持有锁。
func saveLocked() error {
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化计划表失败: %w", err)
	}
	if err := persistence.WriteFileAtomic(storageFile, raw); err != nil {
		logger.Warn(Module, "[计划表] 保存失败: %v", err)
		return fmt.Errorf("保存计划表失败: %w", err)
	}
	return nil
}

// findLocked 查找计划项下标；未找到返回 -1。
func findLocked(id string) int {
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}

// generateID 生成 UUID v4 形式的主键。
func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// formatISO 统一时间文本。
func formatISO(value time.Time) string { return value.Format("2006-01-02T15:04:05") }

// formatDate 统一日期文本。
func formatDate(value time.Time) string { return value.Format("2006-01-02") }
