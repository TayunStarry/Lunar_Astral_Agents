package persistence

import (
	"time"
)

// FileInfo 文件信息结构体，用于存储文件和目录的相关信息
type FileInfo struct {
	Name         string    `json:"name"`                   // 文件名或目录名
	Size         int64     `json:"size"`                   // 文件大小
	IsDir        bool      `json:"isDir"`                  // 是否为目录
	LastModified time.Time `json:"lastModified"`           // 最后修改时间
	Path         string    `json:"path"`                   // 相对于配置目录的相对路径
	Handle       string    `json:"handle,omitempty"`       // 句柄（内容仓库内文件；目录或无句柄时为空）
	Kind         string    `json:"kind,omitempty"`         // 资源类型码 D/I/V/A/T
	ContentHash  string    `json:"content_hash,omitempty"` // 媒体内容哈希（文本类为空）
}

// SaveOptions 文件写入选项
type SaveOptions struct {
	// Overwrite 目标已存在时是否覆盖；false 时按时间戳生成新版本
	Overwrite bool
	// MustStore 强制写入指定目录（仅对「新内容」生效）。
	// 内容哈希已存在时仍以唯一性为准：不会因 MustStore 制造第二份实体。
	MustStore bool
}

// SaveResult 文件写入结果（含句柄映射信息）
type SaveResult struct {
	Name    string `json:"name"`     // 最终文件名（覆盖/重命名后）
	Path    string `json:"path"`     // 绝对路径
	RelPath string `json:"rel_path"` // 相对 LocalDir 的路径
	Handle  string `json:"handle"`   // 句柄（内容仓库内文件；否则为空）
	Deduped bool   `json:"deduped"`  // 是否复用了既有内容实体
	Size    int64  `json:"size"`     // 文件字节数
}

// PreviewEntry 文件预览条目，包含 MIME 类型和文件类别
type PreviewEntry struct {
	MIME     string // MIME 类型
	Category string // 文件类别: image / video / text
}

// FileHandle 句柄映射的只读视图（文件列表等展示用）
type FileHandle struct {
	Handle      string
	Kind        string
	ContentHash string
}

// ResourceObserver 资源句柄观察者：底层写 / 移 / 删 / 读原语完成后回调，
// 由 engine/resource 在初始化成功后注册。
//
// 依赖方向为 persistence ← resource（resource 引用本包的文件原语），
// 因此本包只定义接口，不 import resource。
type ResourceObserver interface {
	// AfterWrite 写盘完成后回调，返回最终保留路径与句柄（句柄为空表示未登记）
	AfterWrite(rel string) (canonicalRel, handle string)
	// AfterMove 移动 / 改名完成后回调
	AfterMove(oldRel, newRel string)
	// AfterDelete 删除完成后回调
	AfterDelete(rel string)
	// AfterRead 读取命中后回调（续期 / 清理失效映射）
	AfterRead(rel string)
	// Lookup 按路径查询句柄映射（只读）
	Lookup(rel string) (FileHandle, bool)
}

// OrganizeOperation 单个整理操作
type OrganizeOperation struct {
	Type   string `json:"type"`             // 操作类型: move, rename, merge, delete
	Source string `json:"source"`           // 源路径（相对于工作目录）
	Target string `json:"target,omitempty"` // 目标路径（相对于工作目录），delete 操作不需要
}

// OrganizeResult 单个操作结果
type OrganizeResult struct {
	Success bool   `json:"success"`
	Type    string `json:"type"`
	Source  string `json:"source"`
	Target  string `json:"target,omitempty"`
	Error   string `json:"error,omitempty"`
}

// OrganizeRequest 批量整理请求
type OrganizeRequest struct {
	BasePath   string              `json:"base_path"`  // 工作目录基础路径（相对于 LocalDir）
	Operations []OrganizeOperation `json:"operations"` // 操作列表
}

// OrganizeResponse 批量整理响应
type OrganizeResponse struct {
	Success      bool             `json:"success"`
	Results      []OrganizeResult `json:"results"`
	Total        int              `json:"total"`
	SuccessCount int              `json:"success_count"`
	FailCount    int              `json:"fail_count"`
	Error        string           `json:"error,omitempty"`
}

// =============================================================================
// 文件移动类型定义（/file/move）
// =============================================================================

// MoveItemRequest 文件移动请求
// Sources 为相对 LocalDir 的源路径列表；TargetDir 为目标文件夹（相对 LocalDir，空表示根目录）
// ConflictStrategy 冲突策略: "ask"（预检冲突并返回，不执行移动）/ "auto_rename"（自动添加序号重命名）/ "overwrite"（覆盖同名项）
type MoveItemRequest struct {
	Sources          []string `json:"sources"`           // 源路径列表（相对 LocalDir）
	TargetDir        string   `json:"target_dir"`        // 目标文件夹（相对 LocalDir，空表示根目录）
	ConflictStrategy string   `json:"conflict_strategy"` // 冲突策略: ask / auto_rename / overwrite
	CreateDirs       bool     `json:"create_dirs"`       // 目标文件夹不存在时是否自动创建
}

// MoveConflict 移动冲突项（ask 预检时返回）
type MoveConflict struct {
	Source string `json:"source"` // 源路径
	Target string `json:"target"` // 目标路径
	IsDir  bool   `json:"is_dir"` // 冲突项是否为文件夹
}

// MoveItemResult 单个移动结果
type MoveItemResult struct {
	Source   string `json:"source"`   // 源路径
	Target   string `json:"target"`   // 最终目标路径（冲突重命名后为实际路径）
	Conflict bool   `json:"conflict"` // 是否检测到同名冲突
	Renamed  bool   `json:"renamed"`  // 是否因冲突自动重命名
	Error    string `json:"error,omitempty"`
}

// MoveResponse 文件移动响应
type MoveResponse struct {
	Success   bool             `json:"success"`
	Results   []MoveItemResult `json:"results,omitempty"`
	Conflicts []MoveConflict   `json:"conflicts,omitempty"` // ask 预检冲突列表
	Error     string           `json:"error,omitempty"`
}

// ZipEntryInfo ZIP 压缩包内条目信息
type ZipEntryInfo struct {
	Name       string `json:"name"`       // 条目路径（含子目录）
	Size       int64  `json:"size"`       // 解压后大小
	Compressed int64  `json:"compressed"` // 压缩后大小
	IsDir      bool   `json:"isDir"`      // 是否为目录
}

// ZipMetadataRequest ZIP 元数据查询请求
type ZipMetadataRequest struct {
	Path string `json:"path"` // ZIP 文件路径（相对 LocalDir）
}

// ZipMetadataResponse ZIP 元数据查询响应
type ZipMetadataResponse struct {
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Path      string         `json:"path"`       // 压缩包相对 LocalDir 的路径
	FileCount int            `json:"file_count"` // 条目总数
	TotalSize int64          `json:"total_size"` // 解压后总大小
	ZipSize   int64          `json:"zip_size"`   // 压缩包文件大小
	Entries   []ZipEntryInfo `json:"entries"`    // 条目列表
}

// ExtractZipRequest ZIP 解压请求
type ExtractZipRequest struct {
	Path      string `json:"path"`       // ZIP 文件路径（相对 LocalDir）
	TargetDir string `json:"target_dir"` // 目标目录（相对 LocalDir，空表示 LocalDir 根目录）
}

// ExtractZipResponse ZIP 解压响应
type ExtractZipResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	TargetDir string `json:"target_dir"` // 实际解压目录（相对 LocalDir）
	FileCount int    `json:"file_count"` // 解压出的文件数
}

// CreateZipRequest 服务端压缩请求
type CreateZipRequest struct {
	Paths    []string `json:"paths"`     // 相对 LocalDir 的文件/目录路径列表
	ZipName  string   `json:"zip_name"`  // ZIP 文件名（缺省为 archive.zip，自动补全 .zip 后缀）
	SavePath string   `json:"save_path"` // 保存目录（相对 LocalDir，空表示 LocalDir 根目录）
}

// ZipCreateResponse 服务端压缩响应
type ZipCreateResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Path    string `json:"path"` // 压缩包相对 LocalDir 的路径
	Name    string `json:"name"` // 压缩包文件名
	Size    int    `json:"size"` // 压缩包大小（字节）
}

// HashRenameRequest 哈希命名请求
type HashRenameRequest struct {
	Path string `json:"path"` // 目标目录路径（相对 LocalDir，空表示根目录）
}

// HashRenameItem 单个文件的哈希命名结果
type HashRenameItem struct {
	OldName   string `json:"old_name"`  // 原文件名
	NewName   string `json:"new_name"`  // 新文件名（MD5 前 16 位 + 原扩展名）
	Hash      string `json:"hash"`      // 文件内容 MD5 的前 16 位
	Duplicate bool   `json:"duplicate"` // 是否因重名追加了 '+'
	Unchanged bool   `json:"unchanged"` // 是否无需改动（已是哈希名）
}

// HashRenameResponse 哈希命名响应
type HashRenameResponse struct {
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
	Results []HashRenameItem `json:"results"` // 全部文件的处理结果
	Renamed int              `json:"renamed"` // 实际重命名的文件数
}
