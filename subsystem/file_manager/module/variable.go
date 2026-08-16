package module

import "sync"

// 文件类别常量
const (
	CategoryImage = "image"
	CategoryVideo = "video"
	CategoryText  = "text"
)

// 记忆库集合类型常量
const (
	CollectionTypeText  = "text"  // 文本集合类型（documents_*.json + tags_*.json 分块存储）
	CollectionTypeImage = "image" // 图片集合类型（images_*.json + tags_*.json 分块存储）
)

// v2 记忆库分块大小常量
const (
	DocumentsChunkSize = 500 // text 文档分块大小（条/块）
	ImagesChunkSize    = 20  // image 文档分块大小（条/块）
	TagsChunkSize      = 100 // 标签向量分块大小（条/块）
)

// v3 记忆库配置常量
const (
	CurrentVersion    = 3    // 当前数据格式版本号（v3: 文档引用标签 UUID）
	TagDedupThreshold = 0.85 // 标签向量去重阈值（余弦相似度）
	MaxTagRetries     = 3    // LLM 标签生成最大重试次数
)

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

// MemoryDatabase 记忆库实例（MemoryDB）
var MemoryDatabase *MemoryDB
