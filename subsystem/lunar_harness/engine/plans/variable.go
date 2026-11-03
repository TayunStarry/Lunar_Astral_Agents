package plans

// variable.go 计划表常量、正则与全局状态（业务逻辑不得写在本文件）。

import (
	"regexp"
	"sync"
	"time"
)

const (
	// Module 日志模块名
	Module = "PlanManager"
	// storageFile 计划表存储路径（相对 LocalDir）
	storageFile = "database/schedule.json"
)

// 日期解析规则
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

// presetDailyTasks 默认每日计划项。
var presetDailyTasks = []Item{
	{ID: "daily_greeting_morning", Type: "daily", Time: "07:30", Content: "向用户发送早安问候，并提示可以查一下今天的天气"},
	{ID: "daily_greeting_evening", Type: "daily", Time: "22:00", Content: "向用户发送晚安问候，祝其好眠，并提示可以查看一下今天的新闻"},
}

var (
	// mu 保护计划表内存缓存
	mu sync.Mutex
	// items 计划表内存缓存
	items []Item
)

// 时间布局（用于 NormalizeTime）
var timeLayouts = []string{
	"2006-01-02T15:04:05", "2006-01-02T15:04", time.RFC3339,
	"2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006/1/2 15:04:05", "2006/1/2 15:04",
}
