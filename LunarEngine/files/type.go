package files

import "time"

// FileInfo 文件信息结构体，用于存储文件和目录的相关信息
type FileInfo struct {
	Name         string    `json:"name"`         // 文件名或目录名
	Size         int64     `json:"size"`         // 文件大小
	IsDir        bool      `json:"isDir"`        // 是否为目录
	LastModified time.Time `json:"lastModified"` // 最后修改时间
	Path         string    `json:"path"`         // 相对于配置目录的相对路径
}
