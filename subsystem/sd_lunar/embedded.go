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
	subFS, err := fs.Sub(EmbeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
