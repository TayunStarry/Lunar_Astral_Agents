package module

import "sync"

// 文件类别常量
const (
	CategoryImage = "image"
	CategoryVideo = "video"
	CategoryText  = "text"
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

// SQLDatabase 关系型数据库实例（SQLDB）
var SQLDatabase *SQLDB

// VectorDatabase 向量数据库实例（VectorDB）
var VectorDatabase *VectorDB
