package resource

// variable.go 资源句柄系统的全局常量与变量
//
// 本包是句柄文法、类型码、目录布局、段落表与 GC 规则在 Go 侧的**唯一权威实现**：
// 智能体与前端只引用这里导出的接口，不再各自维护第二份判定。

import (
	"regexp"
	"sync"
)

// 资源类型码
const (
	KindDocument = "D" // 文档（文本类文件：源码 / Markdown / 纯文本 / 配置）
	KindImage    = "I" // 图片
	KindVideo    = "V" // 视频
	KindAudio    = "A" // 音频
	KindTable    = "T" // 表格（预留）
	KindURL      = "U" // 链接（网页 / 网络视频 / BV 号：只记录地址与缓存摘要，不下载本体）
)

// 资源生命周期状态（blob 存在性）
const (
	// StateActive blob 在位，可直接取回本体
	StateActive = "active"
	// StateLost blob 已被回收或缺失，句柄仍可解析，取用时降级为描述/大纲
	StateLost = "lost"
)

// 保留等级（决定 GC 顺序，等级相同再按最久未用）
const (
	// RetainNormal A 级：用户主动提供，永久保留
	RetainNormal = "normal"
	// RetainSoft B 级：引擎产出，可被回收（回收后降级为描述）
	RetainSoft = "soft"
	// RetainEphemeral C 级：临时参考物（视频抽帧、GIF 中间帧），最优先回收
	RetainEphemeral = "ephemeral"
)

// 资源来源（决定默认保留等级与入库行为）
const (
	OriginClientUpload = "client-upload" // 前端上传（已落盘，只登记不复制）
	OriginChannel      = "channel"       // QQ 等外部渠道（仅 URL，需下载落盘）
	OriginToolOutput   = "tool-output"   // 工具产出（截图 / 绘图 / 超分）
	OriginEngineTemp   = "engine-temp"   // 引擎临时媒体（视频分段 / 抽帧）
)

// OriginStorageScan 启动扫描入库的来源标记
const OriginStorageScan = "storage-scan"

// 句柄文法正则：唯一权威定义，TypeScript 与前端通过 resourceGrammar 取用，禁止各处自写
const (
	// HandlePattern 完整句柄（含可选锚点）
	HandlePattern = `@([DIVATU])\.(\d{3,})(?:\|(?:§(\d+)|L(\d+)-(\d+)))?`
	// SeqPadWidth 句柄序号最小宽度（不足补零）
	SeqPadWidth = 3
	// seqOverflowWidth 序号超出三位后的宽度（999 之后自然变为四位）
	seqOverflowWidth = 4
)

// 句柄体系错误语义（调用方与 HTTP 层按同一组文案透出）
const (
	ErrHandleInvalid = "句柄无效,文件不存在"
	ErrFileLost      = "文件损毁或丢失,请更换句柄"
)

// URLMIME 链接资源在目录中的 MIME 标记（便于前端识别，无实际字节）
const URLMIME = "text/uri-list"

// MaxReadBytes 单次取回内容的字节上限（防止一次读入超大文件撑爆上下文）
const MaxReadBytes = 256 * 1024

// SectionBlockLines 无标题文本退化为段落时的行块大小
const SectionBlockLines = 60

// 来源类型
const (
	SourceKindPath      = "path"      // 本地文件路径（相对 LocalDir 或绝对路径或 file:// 或本机 http(s):// 服务地址）
	SourceKindURL       = "url"       // 远程地址
	SourceKindData      = "data"      // data URI
	SourceKindBytes     = "bytes"     // 原始字节
	SourceKindWorkspace = "workspace" // 工作区文件（用户直接引用的项目内文件，按只读方式收录）
)

// workspaceMaxBytes 单个工作区文件收录的体积上限（100MB，与工作区大文件留出余量）
const workspaceMaxBytes = 100 * 1024 * 1024

// 内容仓库根目录（相对 LocalDir）：句柄体系只维护这三棵树
const (
	StorageRootDocuments  = "documents"
	StorageRootAudios     = "audios"
	StorageRootMultimedia = "multimedia"
)

// DerivedStorageRoots 内容仓库内的「派生缓存」子树（相对 LocalDir）。
//
// 这些目录里的文件由引擎按内容寻址生成、可随时重建，不是用户资料：
//   - multimedia/vision：视觉读取副本（缩放 + 句柄叠印），按原图内容哈希缓存；
//   - multimedia/footage：视频解读切片（ffmpeg 按时长分段），按源视频内容哈希缓存；
//   - audios/cache：TTS 合成语音的缓存（按文本与音色键可重建）。
//
// 句柄体系只为「月华需要长期找到并引用的资料」服务；派生缓存既不需要被引用，
// 又会与源文件形成同内容哈希的重复实体（扫描去重甚至会删掉缓存副本），
// 因此它们**不进入**句柄索引：不登记、不分配序号，历史遗留的目录行在启动同步时清理。
var DerivedStorageRoots = []string{
	StorageRootMultimedia + "/vision",
	StorageRootMultimedia + "/footage",
	StorageRootAudios + "/cache",
}

// StorageRoots 内容仓库扫描根（顺序固定，便于日志与测试）
var StorageRoots = []string{StorageRootDocuments, StorageRootAudios, StorageRootMultimedia}

// handleExactRegexp 整串句柄匹配（解析用）
var handleExactRegexp = regexp.MustCompile("^" + HandlePattern + "$")

// handleGlobalRegexp 文本内句柄提取
var handleGlobalRegexp = regexp.MustCompile(HandlePattern)

// kindLabels 类型码 → 中文类型名
var kindLabels = map[string]string{
	KindDocument: "文档",
	KindImage:    "图片",
	KindVideo:    "视频",
	KindAudio:    "音频",
	KindTable:    "表格",
	KindURL:      "链接",
}

// mediaExtensions 媒体类扩展名（决定归档副本落在媒体树还是文档树）
var mediaExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".svg": true, ".ico": true, ".apng": true,
	".mp4": true, ".webm": true, ".avi": true, ".mov": true, ".mkv": true, ".wmv": true,
	".flv": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".wav": true, ".mp3": true, ".flac": true, ".ogg": true, ".m4a": true, ".aac": true,
	".opus": true, ".wma": true,
}

// workspaceTextExtensions 允许按需收录的工作区文本类扩展名（白名单，防止把可执行文件等读入归档）
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

var (
	// archiveInitMu 串行化资源目录初始化（HTTP 服务先于人格层启动，存在并发首访）
	archiveInitMu sync.Mutex
	// archiveMu 串行化序号分配与目录写入，避免并发下句柄重号
	archiveMu sync.Mutex
	// archiveSeqCache 各类型码的下一个可用序号缓存（启动时从 resources 表重建）
	archiveSeqCache = map[string]int64{}
	// archiveSeqLoaded 序号缓存是否已从库中重建
	archiveSeqLoaded bool
	// storageScanMu 串行化启动同步，避免并发初始化重复建行
	storageScanMu sync.Mutex
	// archiveDB 统一资源目录实例（句柄 / 元数据 / 段落结构）
	archiveDB *store
)
