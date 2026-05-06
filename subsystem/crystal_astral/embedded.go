package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
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
