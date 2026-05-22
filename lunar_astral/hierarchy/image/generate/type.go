package generate

import (
	"sync"
	"time"
)

// GenerateTask 生成任务结构体
type GenerateTask struct {
	ID             string    `json:"id"`
	Prompt         string    `json:"prompt"`
	NegativePrompt string    `json:"negative_prompt"`
	BatchSize      int       `json:"batch_size"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	Strength       float64   `json:"strength"`
	Steps          int       `json:"steps"`
	Seed           int64     `json:"seed"`
	CfgScale       float64   `json:"cfg_scale"`
	InitImg        string    `json:"init_img"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
	ResultPath     string    `json:"result_path"`
	Error          string    `json:"error"`
}

var (
	TaskQueue     = make(chan GenerateTask, 10)
	TaskStatus    = make(map[string]*GenerateTask)
	TaskStatusMu  sync.RWMutex
	WaitClients   = make(map[string]chan *GenerateTask)
	WaitClientsMu sync.RWMutex
)