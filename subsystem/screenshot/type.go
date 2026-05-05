package screenshot

import "time"

// ScreenshotRequest 截图请求参数
type ScreenshotRequest struct {
	DisplayIndex int    `json:"display_index"` // -1表示所有显示器
	Region       string `json:"region"`        // "x,y,width,height"
	Scale        string `json:"scale"`         // "width,height" 或 "0.5"
	Format       string `json:"format"`        // png, jpg, jpeg
	Quality      int    `json:"quality"`       // JPEG质量 1-100
}

// 最后截图时间和频率限制
var (
	lastCapture     time.Time               // 最后截图时间
	captureCooldown = 50 * time.Millisecond // 最小截图间隔
)