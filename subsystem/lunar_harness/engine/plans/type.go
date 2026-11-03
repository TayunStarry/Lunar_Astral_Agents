package plans

// type.go 计划表领域类型。

import "time"

// Item 计划表项（与既有 database/schedule.json 字段保持一致）。
type Item struct {
	ID            string `json:"id"`
	Time          string `json:"time"`
	Content       string `json:"content"`
	Type          string `json:"type,omitempty"`
	CompletedDate string `json:"completedDate,omitempty"`
}

// dateRange 查询日期范围。
type dateRange struct {
	Start time.Time
	End   time.Time
	Label string
}
