package filesupport

// type.go 文件操纵共享的版本记录与路径约定。

// Version 一个文件版本。
type Version struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Op 触发这次备份的操作类型（replace / append / delete_lines / create / uninstall 等）
	Op        string `json:"op,omitempty"`
	Exists    bool   `json:"exists"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	BackupRel string `json:"backup_rel,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// VersionStats 版本库的总体占用情况。
type VersionStats struct {
	// Files 有历史版本的文件数
	Files int64 `json:"files"`
	// Versions 版本总数
	Versions int64 `json:"versions"`
	// Bytes 版本内容总字节数
	Bytes int64 `json:"bytes"`
}
