package lunar_window

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed resource/*
var EmbeddedFiles embed.FS

// GetResourceFS 返回包含背景图片所在的 resource 目录文件系统
func GetResourceFS() http.FileSystem {
	subFS, err := fs.Sub(EmbeddedFiles, "resource")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
