package image

import (
	"sync"
	"time"
)

// GenerateTask 生成任务结构体
type GenerateTask struct {
	// ID 任务ID
	ID string `json:"id"`
	// Prompt 提示词
	Prompt string `json:"prompt"`
	// NegativePrompt 负提示词
	NegativePrompt string `json:"negative_prompt"`
	// BatchSize 批量大小
	BatchSize int `json:"batch_size"`
	// Width 宽度
	Width int `json:"width"`
	// Height 高度
	Height int `json:"height"`
	// Strength 强度
	Strength float64 `json:"strength"`
	// Steps 步骤
	Steps int `json:"steps"`
	// Seed 种子
	Seed int64 `json:"seed"`
	// CfgScale 配置缩放
	CfgScale float64 `json:"cfg_scale"`
	// InitImg 初始化图像
	InitImg string `json:"init_img"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// Status 状态
	Status string `json:"status"`
	// ResultPath 结果路径
	ResultPath string `json:"result_path"`
	// Error 错误信息
	Error string `json:"error"`
}

// TaskQueue 生成任务队列
var TaskQueue = make(chan GenerateTask, 10)

// TaskStatus 任务状态映射
var TaskStatus = make(map[string]*GenerateTask)

// TaskStatusMu 任务状态映射互斥锁
var TaskStatusMu sync.RWMutex

// WaitClients 等待任务映射
var WaitClients = make(map[string]chan *GenerateTask)

// WaitClientsMu 等待任务映射互斥锁
var WaitClientsMu sync.RWMutex
