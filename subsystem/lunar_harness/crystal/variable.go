package crystal

import (
	"sync"
	"time"
)

// ==== 全局变量 ====

// ==== LTPX 待定调用（前端包经 /ws 派发、回执经 /ltpx/result 回填） ====

// ltpPendingMutex 保护待定调用注册表的并发读写
var ltpPendingMutex sync.Mutex

// ltpPendingCalls 待定调用注册表：request_id → 待定调用条目
// 调用方经进程内结果通道取回结果（Broker.Await）；回执/内置智能体完成后写入结果通道，
// 超时未取走时由 schedulePendingCallCleanup 投递失败结果并清理条目
var ltpPendingCalls = map[string]*ltpPendingCall{}

// ltpCallTimeout 等待前端包执行工具结果的最大时长（LTP9 前端智能体调用路径使用）
const ltpCallTimeout = 120 * time.Second

// ltpPendingMaxAge 待定调用最长保留时长：超时后投递失败结果并清理，防止注册表泄漏
const ltpPendingMaxAge = 30 * time.Minute

// ==== LTPX 前端守护（唤起器/探针由装配层注入） ====

// frontendGuardMu 保护唤起流程的单飞互斥（并发 LTPX 派发共享同一次唤起）
var frontendGuardMu sync.Mutex

// frontendRelauncher 琉璃前端唤起器：重建琉璃桌面窗口（窗口已存在时聚焦）
var frontendRelauncher func() error

// frontendReadyProbe 琉璃前端就绪探针：缺省频道存在总线订阅者即视为在线
// （琉璃桌面以 /ws?bus=1 缺省频道接入，月华侧引擎 iframe 已显式 bus=0 退订）
var frontendReadyProbe func() bool

const (
	// frontendReviveTimeout 唤起后等待前端重新接入总线的最长时长
	frontendReviveTimeout = 20 * time.Second
	// frontendRevivePollInterval 就绪探针轮询间隔
	frontendRevivePollInterval = 500 * time.Millisecond
)

// fileCategoryMap 文件扩展名到分类的映射
var fileCategoryMap = map[string]string{
	// 文本文件
	".txt": "text", ".md": "text", ".log": "text", ".csv": "text",
	".json": "text", ".xml": "text", ".yaml": "text", ".yml": "text",
	".toml": "text", ".ini": "text", ".cfg": "text", ".conf": "text",
	".html": "text", ".css": "text", ".js": "text", ".ts": "text",
	".go": "code", ".py": "code", ".java": "code", ".c": "code",
	".cpp": "code", ".h": "code", ".rs": "code", ".rb": "code",
	".sh": "code", ".bat": "code", ".ps1": "code",
	// 图片文件
	".png": "image", ".jpg": "image", ".jpeg": "image", ".webp": "image",
	".gif": "image", ".svg": "image", ".bmp": "image", ".ico": "image",
	".tiff": "image", ".tif": "image", ".avif": "image",
	// 视频文件
	".mp4": "video", ".webm": "video", ".avi": "video", ".mov": "video",
	".mkv": "video", ".wmv": "video", ".flv": "video", ".m4v": "video",
	".mpg": "video", ".mpeg": "video",
	// 音频文件
	".mp3": "audio", ".wav": "audio", ".flac": "audio", ".aac": "audio",
	".ogg": "audio", ".wma": "audio", ".m4a": "audio",
	// 压缩文件
	".zip": "archive", ".rar": "archive", ".7z": "archive", ".tar": "archive",
	".gz": "archive", ".bz2": "archive", ".xz": "archive",
}
