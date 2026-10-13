package crystal

import (
	"embed"
	"sync"
	"time"
)

// ==== 记忆库自动初始化默认值 ====
// 与月华的 TypeScript 默认值保持一致
const (
	// DefaultMemoryModelName 默认嵌入模型名
	DefaultMemoryModelName = "system-embedding"
	// DefaultMemoryCollection 记忆库前端操作的固定集合名
	DefaultMemoryCollection = "lunar_messages"
)

// ==== 全局变量 ====

// EmbeddedFiles 嵌入的琉璃桌面静态资源文件系统（挂载于 /desktop/）
//
//go:embed web/*
var EmbeddedFiles embed.FS

// ==== LTPX 动态工具链（来自包的 AtoA 能力） ====

// ltpToolchainMutex 保护工具链注册表与工具名→包名映射的并发读写
var ltpToolchainMutex sync.RWMutex

// ltpAtoaTools 从包元数据扫描收集的「智能体式」工具链（随包增删动态变化）
var ltpAtoaTools []LTPXRemoteToolDef

// ltpToolPackageMap 工具名 → 提供该工具的包目录名（用于路由到对应包）
var ltpToolPackageMap = map[string]string{}

// ==== LTPX 待定调用（转发前端包执行） ====

// ltpPendingMutex 保护待定调用注册表的并发读写
var ltpPendingMutex sync.Mutex

// ltpPendingCalls 待定调用注册表：request_id → 待定调用条目
// 调用方经进程内结果通道或 /ltpx/poll 取回执行状态，回执/内置智能体完成后写入结果通道
var ltpPendingCalls = map[string]*ltpPendingCall{}

// ltpCallTimeout 等待前端包执行工具结果的最大时长
const ltpCallTimeout = 120 * time.Second

// ltpPendingMaxAge 待定调用最长保留时长：超时后判定失败并清理，防止注册表泄漏
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
