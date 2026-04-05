package files

import (
	"embed"    // 引入 embed 包，用于嵌入静态资源
	"io/fs"    // 引入 fs 包，用于操作文件系统
	"net/http" // 引入 http 包，用于构建 HTTP 客户端和服务器
)

//go:embed assets/*
var embeddedFiles embed.FS

// GetFileSystem 返回嵌入的文件系统
func GetFileSystem() http.FileSystem {
	// 创建一个子文件系统，只包含assets目录下的内容
	subFS, err := fs.Sub(embeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
