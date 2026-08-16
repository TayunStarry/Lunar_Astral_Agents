package module

import (
	"image"
	"time"
)

// KeyFrame 关键帧结构
type KeyFrame struct {
	// 关键帧文件名
	FilePath string `json:"filePath"`
	// 关键帧时间戳
	Timestamp string `json:"timestamp"`
	// 关键帧编号
	FrameNum int `json:"frameNum"`
	// 关键帧图像数据
	Data []byte `json:"data,omitempty"`
}

// FrameData 存储帧数据和时间戳
type FrameData struct {
	Image     image.Image
	Timestamp int
}

// CaptureMode 截图模式
type CaptureMode string

const (
	// ModeAuto 默认模式：优先捕获焦点窗口，识别失败时降级为多屏拼接全屏
	ModeAuto CaptureMode = "auto"
	// ModeWindow 强制捕获焦点窗口，识别失败直接报错
	ModeWindow CaptureMode = "window"
	// ModeFullscreen 多屏拼接全屏截图
	ModeFullscreen CaptureMode = "fullscreen"
	// ModeDisplay 指定显示器截图
	ModeDisplay CaptureMode = "display"
	// ModeRegion 绝对屏幕坐标区域截图
	ModeRegion CaptureMode = "region"
)

// CaptureRequest 统一截图请求
type CaptureRequest struct {
	Mode         CaptureMode `json:"mode"`          // 截图模式，默认 auto
	DisplayIndex int         `json:"display_index"` // mode=display 时生效，-1 表示全部

	// 窗口相对精准区域（mode=auto/window）
	// 触发条件：Width>0 && Height>0；OffsetX/OffsetY 缺省为 0（窗口左上角）
	OffsetX int `json:"offset_x"`
	OffsetY int `json:"offset_y"`
	Width   int `json:"width"`
	Height  int `json:"height"`

	// 绝对屏幕坐标区域（mode=region）
	RegionX int `json:"region_x"`
	RegionY int `json:"region_y"`
	RegionW int `json:"region_w"`
	RegionH int `json:"region_h"`

	// 输出参数
	Format  string `json:"format"`  // png / jpeg，默认取 general_config
	Quality int    `json:"quality"` // JPEG 质量 1-100，默认取 general_config
	Scale   string `json:"scale"`   // 可选缩放：比例（0.5）或指定宽高（800,600），仅 HTTP 直出路径使用
}

// CaptureResult 统一截图结果
type CaptureResult struct {
	Image        []byte      `json:"-"`                // 原始图像字节（HTTP 直出）
	Format       string      `json:"format"`           // png / jpeg
	ContentType  string      `json:"content_type"`     // image/png / image/jpeg
	Width        int         `json:"width"`            // 最终宽度（缩放后）
	Height       int         `json:"height"`           // 最终高度（缩放后）
	Mode         CaptureMode `json:"mode"`             // 实际采用的模式（含降级后）
	DisplayIndex int         `json:"display_index,omitempty"`
	WindowTitle  string      `json:"window_title,omitempty"` // 焦点窗口标题（window 模式）
}

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
	// AllowSuperResolution 允许超分
	AllowSuperResolution bool `json:"allow_super_resolution"`
}
