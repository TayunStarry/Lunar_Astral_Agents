package multimodal

import (
	"os"
	"sync"
	"time"
)

// engineLogWriter 绘图引擎（sd.cpp）输出流的分行写入器：
// 引擎按 1KB 块输出、进度用 \r 原地刷新，不切行会让宿主收到半截行而无法排版；
// 未注入日志钩子时（桌面构建）保持原始直写，进度条行为与之前一致。
type engineLogWriter struct {
	// stream 输出流标识（stdout / stderr）
	stream string
	// fallback 未注入钩子时的直写目标（原始终端流）
	fallback *os.File
	// buf 未成行的残余字节
	buf []byte
	// mu 保护 buf 与 vaeFallbackWarned（读取侧为单协程，此处为防御性加锁）
	mu sync.Mutex
	// vaeFallbackWarned 本次引擎进程是否已报告过「VAE 解码被自行降级为分片」
	// （一次生成里引擎会连打数行，只提示第一行即可）
	vaeFallbackWarned bool
}

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

	// 目标窗口（mode=auto/window）
	// WindowHandle 优先于 WindowTitle；两者都为空时截取当前焦点窗口。
	// 指定目标后不再要求窗口处于前台：走 PrintWindow 渲染，可截到被遮挡但未最小化的窗口。
	WindowHandle uint64 `json:"window_handle"`
	WindowTitle  string `json:"window_title"`
	// ClientArea 是否只截客户区（去掉标题栏与边框），默认 false（整个窗口）
	ClientArea bool `json:"client_area"`

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
	Image        []byte      `json:"-"`            // 原始图像字节（HTTP 直出）
	Format       string      `json:"format"`       // png / jpeg
	ContentType  string      `json:"content_type"` // image/png / image/jpeg
	Width        int         `json:"width"`        // 最终宽度（缩放后）
	Height       int         `json:"height"`       // 最终高度（缩放后）
	Mode         CaptureMode `json:"mode"`         // 实际采用的模式（含降级后）
	DisplayIndex int         `json:"display_index,omitempty"`
	WindowTitle  string      `json:"window_title,omitempty"`  // 实际捕获的窗口标题（window 模式）
	WindowHandle uint64      `json:"window_handle,omitempty"` // 实际捕获的窗口句柄（window 模式）
}

// WindowInfo 可截图的顶层窗口信息（截图目标枚举结果）
type WindowInfo struct {
	Handle     uint64 `json:"handle"`     // 窗口句柄
	Title      string `json:"title"`      // 窗口标题
	Class      string `json:"class"`      // 窗口类名
	ProcessID  uint32 `json:"pid"`        // 进程 ID
	Process    string `json:"process"`    // 进程可执行文件名
	X          int    `json:"x"`          // 窗口矩形左上角 X（屏幕坐标）
	Y          int    `json:"y"`          // 窗口矩形左上角 Y（屏幕坐标）
	Width      int    `json:"width"`      // 窗口矩形宽度
	Height     int    `json:"height"`     // 窗口矩形高度
	ClientW    int    `json:"client_w"`   // 客户区宽度
	ClientH    int    `json:"client_h"`   // 客户区高度
	Minimized  bool   `json:"minimized"`  // 是否最小化（最小化窗口无法截图）
	Foreground bool   `json:"foreground"` // 是否当前焦点窗口
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
	// Scheme 本任务使用的绘图模型方案（创建任务时从 Go 层当前方案取值，
	// 之后即便切换方案也不影响已入队任务）
	Scheme string `json:"scheme"`
}
