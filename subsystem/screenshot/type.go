package screenshot

// ScreenshotRequest 截图请求参数
type ScreenshotRequest struct {
	DisplayIndex int    `json:"display_index"` // -1表示所有显示器
	Region       string `json:"region"`        // "x,y,width,height"
	Scale        string `json:"scale"`         // "width,height" 或 "0.5"
	Format       string `json:"format"`        // png, jpg, jpeg
	Quality      int    `json:"quality"`       // JPEG质量 1-100
}
