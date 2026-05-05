package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed icon/*
//go:embed assets/*
//go:embed background/*
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

// GetBackgroundFS 返回包含背景图片所在的 background 目录文件系统
func GetBackgroundFS() http.FileSystem {
	subFS, err := fs.Sub(EmbeddedFiles, "background")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}

// GetIconFS 返回包含图标文件的 icon 目录文件系统
func GetIconFS() http.FileSystem {
	subFS, err := fs.Sub(EmbeddedFiles, "icon")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
