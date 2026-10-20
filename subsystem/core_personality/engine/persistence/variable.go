package persistence

import "sync"

// 文件类别常量
const (
	CategoryImage = "image"
	CategoryVideo = "video"
	CategoryText  = "text"
)

// 内容仓库根目录（相对 LocalDir）：句柄体系只维护这三棵树
const (
	StorageRootDocuments  = "documents"
	StorageRootAudios     = "audios"
	StorageRootMultimedia = "multimedia"
)

// MemoryMediaSubdir 记忆库图片在 multimedia 下的专属子目录：
// 图片实体统一落在 multimedia/memory/<集合名>/，与生成图、背景图、截图等隔离，便于清理。
const MemoryMediaSubdir = "memory"

// StorageRoots 内容仓库扫描根（顺序固定，便于日志与测试）
var StorageRoots = []string{StorageRootDocuments, StorageRootAudios, StorageRootMultimedia}

// 句柄体系错误语义（调用方与 HTTP 层按同一组文案透出）
const (
	ErrHandleInvalid = "句柄无效,文件不存在"
	ErrFileLost      = "文件损毁或丢失,请更换句柄"
)

// OriginStorageScan 启动扫描入库的来源标记
const OriginStorageScan = "storage-scan"

// v5 记忆库配置常量 — 文本/图片/音频混合存储，仅内容嵌入向量
const (
	CurrentVersion       = 5    // 当前数据格式版本号（v5: 混合存储 + 仅内容嵌入）
	DocumentsChunkSize   = 100  // 文档分块大小（条/块，各类型统一；100 条一块）
	MemoryDedupThreshold = 0.98 // 内容嵌入向量去重阈值（余弦相似度，判定重复记忆）
	DocRankThreshold     = 0.30 // 文档内容余弦得分门槛（低于此值不返回）
	MaxTagRetries        = 3    // 图片多模态标签生成最大重试次数
)

// 记忆类型 — 查询与列举时按类型过滤，不再按集合名过滤
const (
	MemoryTypeText  = "text"  // 文本记忆（对话原文、知识片段）
	MemoryTypeImage = "image" // 图像记忆（表情包、截图、图片素材）
	MemoryTypeAudio = "audio" // 音频记忆（语音、音乐、环境声）
)

// MemoryTypes 全部合法记忆类型（顺序固定，便于错误提示与遍历）
var MemoryTypes = []string{MemoryTypeText, MemoryTypeImage, MemoryTypeAudio}

// DefaultMemoryCollections 默认分组集合名 — 覆盖聊天记忆可能对应的方方面面。
//
// 集合不再承担“查询表”职责，只是内容归组：启动时确保这些组存在并逐一用集合名
// 生成嵌入向量；入库时把内容向量与全部集合名向量比较，归入最相关的一组。
var DefaultMemoryCollections = []string{
	// 对话与人物
	"日常闲聊",
	"人物信息",
	"关系互动",
	"情感心情",
	"兴趣爱好",
	"偏好习惯",
	"观点态度",
	"动作行为",
	// 经历与事务
	"事件经历",
	"计划日程",
	"工作学习",
	"知识见闻",
	"地点场景",
	// 媒体分组（按查询类型取用，集合本身只是语义分组）
	"表情包",
	"图片素材",
	"截图",
	"绘画作品",
	"语音消息",
	"音乐音频",
	"环境声音",
}

// 图片识别取向常量 — 决定图片标签生成的描述角度与方式
const (
	RecognitionAuto       = "auto"       // 自动处理（默认）：综合性系统描述，由多模态模型自主决定重点
	RecognitionEmotion    = "emotion"    // 情绪表达：专注识别图片所表达的情绪
	RecognitionText       = "text"       // 文本内容：专注识别图片中的文字信息
	RecognitionColor      = "color"      // 色彩风格：专注分析主要配色、次要配色及点缀色
	RecognitionAppearance = "appearance" // 衣着发型：着重人物衣着、发型、身材、发色及瞳色
	RecognitionSpecies    = "species"    // 物种识别：着重事物种类与关键识别特征
	RecognitionPosture    = "posture"    // 姿态动作：重点表达肢体动作及人物表情
	RecognitionCustom     = "custom"     // 自定义：以用户文本作为识别取向参考
)

// FileLocks 用于存储文件路径对应的互斥锁
var FileLocks sync.Map

// 允许预览的文件类型白名单（含分类与 MIME 信息，三合一）
var previewAllowlist = map[string]PreviewEntry{
	// 图片格式
	".png":  {MIME: "image/png", Category: CategoryImage},
	".jpg":  {MIME: "image/jpeg", Category: CategoryImage},
	".jpeg": {MIME: "image/jpeg", Category: CategoryImage},
	".webp": {MIME: "image/webp", Category: CategoryImage},
	".gif":  {MIME: "image/gif", Category: CategoryImage},
	".svg":  {MIME: "image/svg+xml", Category: CategoryImage},
	".ico":  {MIME: "image/x-icon", Category: CategoryImage},
	".bmp":  {MIME: "image/bmp", Category: CategoryImage},
	".tiff": {MIME: "image/tiff", Category: CategoryImage},
	".tif":  {MIME: "image/tiff", Category: CategoryImage},
	// 视频格式
	".mp4":  {MIME: "video/mp4", Category: CategoryVideo},
	".webm": {MIME: "video/webm", Category: CategoryVideo},
	".avi":  {MIME: "video/x-msvideo", Category: CategoryVideo},
	".mov":  {MIME: "video/quicktime", Category: CategoryVideo},
	".mkv":  {MIME: "video/x-matroska", Category: CategoryVideo},
	".wmv":  {MIME: "video/x-ms-wmv", Category: CategoryVideo},
	".flv":  {MIME: "video/x-flv", Category: CategoryVideo},
	".m4v":  {MIME: "video/mp4", Category: CategoryVideo},
	".mpg":  {MIME: "video/mpeg", Category: CategoryVideo},
	".mpeg": {MIME: "video/mpeg", Category: CategoryVideo},
	// 文本格式（仅数据与配置文件，不含代码/脚本）
	".txt":  {MIME: "text/plain", Category: CategoryText},
	".md":   {MIME: "text/markdown", Category: CategoryText},
	".log":  {MIME: "text/plain", Category: CategoryText},
	".csv":  {MIME: "text/csv", Category: CategoryText},
	".json": {MIME: "application/json", Category: CategoryText},
	".xml":  {MIME: "application/xml", Category: CategoryText},
	".yaml": {MIME: "text/yaml", Category: CategoryText},
	".yml":  {MIME: "text/yaml", Category: CategoryText},
	".toml": {MIME: "text/toml", Category: CategoryText},
	".ini":  {MIME: "text/plain", Category: CategoryText},
	".cfg":  {MIME: "text/plain", Category: CategoryText},
}

// KnowledgeDatabase 知识库实例（KnowledgeDB）
var KnowledgeDatabase *KnowledgeDB

// ArchiveDatabase 统一资源目录实例（ArchiveDB，句柄 / 元数据 / 段落结构）
var ArchiveDatabase *ArchiveDB

// MemoryDatabase 记忆库实例（MemoryDB）
var MemoryDatabase *MemoryDB
