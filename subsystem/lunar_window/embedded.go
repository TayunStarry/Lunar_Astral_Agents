package lunar_window

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed resource/*
var EmbeddedFiles embed.FS

// Gethierarchy 返回嵌入的文件系统
func Gethierarchy() http.FileSystem {
	// 创建一个子文件系统，只包含assets目录下的内容
	subFS, err := fs.Sub(EmbeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}

// GetResourceFS 返回包含背景图片所在的 resource 目录文件系统
func GetResourceFS() http.FileSystem {
	subFS, err := fs.Sub(EmbeddedFiles, "resource")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
