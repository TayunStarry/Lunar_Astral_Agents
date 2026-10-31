package multimodal

import "sync"

// 支持的视频格式列表
var supportedVideoFormats = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm", ".m4v"}

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

// 截图互斥锁
var ScreenshotMutex sync.RWMutex

// 最后截图时间和频率限制
var (
	LastCapture     int64                    // 最后截图时间（UnixNano）
	CaptureCooldown int64 = 50 * 1000 * 1000 // 50ms 最小截图间隔（纳秒）
)

// ==== 视觉观看（切片与抽帧） ====
//
// 视频与动图共用同一套规则：视频按 MediaChunkSeconds 秒切片，动图整体为一个切片，
// 每片按内容强度在 16/24/32/48 四档中取帧。数量只在这里定标，视频/动图链路不得各自再定一套。
const (
	// MediaChunkSeconds 单个观看切片的最大时长（秒）
	MediaChunkSeconds = 60.0
	// WatchMaxSlices 单次观看最多处理的切片数（超出的部分不抽帧、不交给模型）
	WatchMaxSlices = 8
	// WatchFrameAuto 帧数自适应哨兵：由运动强度与切片时长在 16/24/32/48 中定档
	WatchFrameAuto = 0
	// WatchFramesMin 单个切片的采样帧数下限（静态长片也不低于此值）
	WatchFramesMin = 16
	// WatchFramesMax 单个切片的采样帧数硬上限（也是定向提问的合并帧上限）
	WatchFramesMax = 48
	// motionProbeDim 运动强度探测帧的最大长边（像素）；只用于定档，不进模型
	motionProbeDim = 96
	// motionQuiet 几乎静止的平均像素差阈值（0-255）
	motionQuiet = 3.0
	// motionCalm 常规运动的阈值
	motionCalm = 10.0
	// motionBusy 快节奏运动的阈值；超过即取最高档
	motionBusy = 22.0
	// longSegmentSeconds 长切片在低运动档位上抬一档的时长阈值（秒）
	longSegmentSeconds = 120.0
	// veryLongSegmentSeconds 超长切片直接取最高档的时长阈值（秒）
	veryLongSegmentSeconds = 300.0
	// MaxMediaWidth 采样帧的最大长边（像素，宽或高均受限）。
	// llama-server 的多模态批处理要求非因果注意力块整体落入单个 ubatch，
	// 高分辨率单帧的视觉 token 数会超出 n_ubatch 上限；长边压到 640px 后每帧落在安全范围。
	MaxMediaWidth = 640
)

// ==== 日志模块标签 ====
//
// 控制台按模块名路由：绘图屏只接收 sd.cpp 绘图链路。视频抽帧、关键帧与
// 图片缩放属于「视觉处理」，混用同一个模块名会把切片消息打进绘图日志。
const (
	// LogModuleDraw 绘图（sd.cpp）编排与引擎日志
	LogModuleDraw = "MultimodalAnalysis"
	// LogModuleMedia 视频抽帧 / 关键帧 / 图片缩放等视觉处理日志（只进原始日志）
	LogModuleMedia = "MediaAnalysis"
)

// ==== 绘图引擎日志钩子 ====

var (
	// engineLogMu 保护引擎日志钩子的并发读写
	engineLogMu sync.RWMutex
	// engineLogHook 绘图引擎（sd.cpp）日志钩子：按行回调，stream 为 stdout/stderr。
	// 宿主（TUI 控制台）注入后由其统一排版显示；未注入时维持直写终端。
	engineLogHook func(stream, line string)
)
