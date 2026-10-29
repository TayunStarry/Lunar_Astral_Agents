package resource

// variable.go 句柄系统的全局常量与变量
//
// 本包是句柄文法、类型码、目录布局、段落表与缓存规则的**唯一权威实现**：
// 智能体与前端只引用这里导出的接口，不再各自维护第二份判定。
//
// 新体系（单库 handles.db，见 store.go）：
//   - url_master  统一 URL 主表：URL ⇄ 解析文本唯一绑定，兼作网页检索缓存
//   - handle_file / handle_image / handle_audio / handle_video / handle_animated / handle_web
//     六张纯映射表：句柄 ⇄ 目标 URL（本地资源统一指向 /file/read/ 路由）
//   - doc_sections 文档段落（按 URL 关联）
//   - cache       统一缓存表：视频抽帧 / 图片缩影 → 源文件 URL
//   - 派生缓存目录（multimedia/vision、multimedia/footage、audios/cache）不入句柄索引

import (
	"regexp"
	"sync"
)

// 资源类型码
const (
	KindDocument = "D" // 文件（文本类：源码 / Markdown / 纯文本 / 配置 / 表格）
	KindImage    = "I" // 图片（静态图）
	KindVideo    = "V" // 视频
	KindAudio    = "A" // 音频
	KindAnimated = "G" // 动图（多帧 GIF / APNG / 动态 WebP）
	KindURL      = "U" // 网页（链接：只记录地址与解析文本）
	// KindTable 历史表格类型码（新体系并入 KindDocument，仅保留常量供旧代码编译判断）
	KindTable = "T"
)

// 资源生命周期状态（本地实体存在性）
const (
	// StateActive 实体在位，可直接取回本体
	StateActive = "active"
	// StateLost 实体已被回收或缺失，句柄仍可解析，取用时降级为描述
	StateLost = "lost"
)

// 解析文本状态
const (
	// ParseNone 尚未解析（use_resource 时才触发解读流程）
	ParseNone = "none"
	// ParseReady 已存在有效解析文本（直接返回，不再触发模型）
	ParseReady = "ready"
	// ParseFailed 解析失败（保留失败标记，允许人工重试）
	ParseFailed = "failed"
)

// 保留等级（决定 GC 顺序，等级相同再按最久未用）
const (
	// RetainNormal A 级：用户主动提供，永久保留
	RetainNormal = "normal"
	// RetainSoft B 级：引擎产出，可被回收（回收后降级为描述）
	RetainSoft = "soft"
	// RetainEphemeral C 级：临时参考物，最优先回收
	RetainEphemeral = "ephemeral"
)

// 资源来源（决定默认保留等级与入库行为）
const (
	OriginClientUpload = "client-upload" // 前端上传（已落盘，只登记不复制）
	OriginChannel      = "channel"       // QQ 等外部渠道（仅 URL，需下载落盘）
	OriginToolOutput   = "tool-output"   // 工具产出（截图 / 绘图 / 超分）
	OriginEngineTemp   = "engine-temp"   // 引擎临时媒体
	OriginStorageScan  = "storage-scan"  // 启动扫描入库
)

// GC 积分与可清理范围（gc_run.go / gc_track.go 共用）
const (
	// GCPointsInit 新建 GC 表 2 记录的初始调用积分
	GCPointsInit = 0
	// GCPointsMin 调用积分硬下限（达到即删除句柄与关联物理文件）
	GCPointsMin = -20
	// GCPointsMax 调用积分硬上限
	GCPointsMax = 20
	// GCPointsUnusedStep 上一轮未使用时的积分变化
	GCPointsUnusedStep = -1
	// GCPointsUsedStep 上一轮被查询使用时的积分变化
	GCPointsUsedStep = 1
	// gcCurrentCallUsed GC 表 2「当前调用」标记：本轮被查询使用过
	gcCurrentCallUsed = 1
	// gcHashAudioNameLen audios 根目录哈希音频的文件名（去扩展名）长度
	gcHashAudioNameLen = 16
)

// 句柄文法正则：唯一权威定义
const (
	// HandlePattern 完整句柄（含可选锚点）
	HandlePattern = `@([DIVAGTU])\.(\d{3,})(?:\|(?:§(\d+)|L(\d+)-(\d+)))?`
	// SeqPadWidth 句柄序号最小宽度（不足补零）
	SeqPadWidth = 3
	// seqOverflowWidth 序号超出三位后的宽度
	seqOverflowWidth = 4
)

// FileReadPrefix 本地资源的目标 URL 前缀（相对路由，不带主机与端口）
const FileReadPrefix = "/file/read/"

// 句柄体系错误语义（调用方与 HTTP 层按同一组文案透出）
const (
	ErrHandleInvalid = "句柄无效,文件不存在"
	ErrFileLost      = "文件损毁或丢失,请更换句柄"
)

// URLMIME 链接资源在目录中的 MIME 标记（便于前端识别，无实际字节）
const URLMIME = "text/uri-list"

// MaxReadBytes 单次取回内容的字节上限
const MaxReadBytes = 256 * 1024

// SectionBlockLines 无标题文本退化为段落时的行块大小
const SectionBlockLines = 60

// 来源类型
const (
	SourceKindPath      = "path"
	SourceKindURL       = "url"
	SourceKindData      = "data"
	SourceKindBytes     = "bytes"
	SourceKindWorkspace = "workspace"
)

// workspaceMaxBytes 单个工作区文件收录的体积上限
const workspaceMaxBytes = 100 * 1024 * 1024

// 内容仓库根目录（相对 LocalDir）
const (
	StorageRootDocuments  = "documents"
	StorageRootAudios     = "audios"
	StorageRootMultimedia = "multimedia"
)

// DerivedStorageRoots 内容仓库内的派生缓存子树（不进入句柄索引）
var DerivedStorageRoots = []string{
	StorageRootMultimedia + "/vision",
	StorageRootMultimedia + "/footage",
	StorageRootAudios + "/cache",
}

// StorageRoots 内容仓库扫描根
var StorageRoots = []string{StorageRootDocuments, StorageRootAudios, StorageRootMultimedia}

// 缓存类型
const (
	// CacheKindVideoSlice 视频切片抽帧缓存（变体为 segN）
	CacheKindVideoSlice = "video-slice"
	// CacheKindImageThumb 图片缩影缓存（变体为边长）
	CacheKindImageThumb = "image-thumbnail"
)

// handleExactRegexp 整串句柄匹配（解析用）
var handleExactRegexp = regexp.MustCompile("^" + HandlePattern + "$")

// handleGlobalRegexp 文本内句柄提取
var handleGlobalRegexp = regexp.MustCompile(HandlePattern)

// kindLabels 类型码 → 中文类型名
var kindLabels = map[string]string{
	KindDocument: "文件",
	KindImage:    "图片",
	KindVideo:    "视频",
	KindAudio:    "音频",
	KindAnimated: "动图",
	KindURL:      "网页",
}

// mediaExtensions 媒体类扩展名
var mediaExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".svg": true, ".ico": true, ".apng": true,
	".mp4": true, ".webm": true, ".avi": true, ".mov": true, ".mkv": true, ".wmv": true,
	".flv": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".wav": true, ".mp3": true, ".flac": true, ".ogg": true, ".m4a": true, ".aac": true,
	".opus": true, ".wma": true,
}

// animatedExtensions 可能承载动图的扩展名（真实是否动图由字节探针判定）
var animatedExtensions = map[string]bool{
	".gif": true, ".webp": true, ".apng": true, ".png": true,
}

// workspaceTextExtensions 允许按需收录的工作区文本类扩展名（白名单）
var workspaceTextExtensions = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".json": true, ".xml": true,
	".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
	".log": true, ".csv": true, ".tsv": true,
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".c": true, ".h": true, ".cpp": true,
	".cxx": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true, ".swift": true,
	".sh": true, ".ps1": true, ".bat": true, ".cmd": true, ".sql": true,
	".html": true, ".htm": true, ".css": true, ".vue": true, ".lua": true, ".r": true,
	".dockerfile": true, ".env": true, ".gitignore": true, ".editorconfig": true,
}

// bvIDPattern B 站视频号（BV + 10 位字母数字）
var bvIDPattern = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)

// qqCDNHosts 腾讯图床 / 媒体域名（只接受 https）
var qqCDNHosts = []string{"gchat.qpic.cn", "multimedia.nt.qq.com.cn"}

// urlTrackingParams 入库规范化时要剔除的跟踪参数
var urlTrackingParams = []string{
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
	"spm_id_from", "vd_source", "from_source", "share_source", "share_medium",
	"share_plat", "share_session_id", "share_tag", "unique_k", "buvid",
}

var (
	// archiveInitMu 串行化资源目录初始化
	archiveInitMu sync.Mutex
	// archiveMu 串行化序号分配与目录写入
	archiveMu sync.Mutex
	// archiveSeqCache 各类型码的下一个可用序号缓存
	archiveSeqCache = map[string]int64{}
	// archiveSeqLoaded 序号缓存是否已从库中重建
	archiveSeqLoaded bool
	// storageScanMu 串行化启动同步
	storageScanMu sync.Mutex
	// archiveDB 统一句柄库实例
	archiveDB *store
)
