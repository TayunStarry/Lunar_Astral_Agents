package server

import (
	"config"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"storage/module"
	"strings"
)

// RandomBackgroundHandler 服务随机选择的背景图片
func RandomBackgroundHandler(w http.ResponseWriter, _ *http.Request) {
	filename, err := module.GetRandomBackgroundImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	backgroundDir := filepath.Join(*config.LocalDir, "images/background")
	filePath := filepath.Join(backgroundDir, filename)
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "无法打开文件: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(filename))
	contentType := ""
	switch ext {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	default:
		contentType = "application/octet-stream"
	}

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "无法获取文件信息: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.WriteHeader(http.StatusOK)

	if _, err := module.CopyBuffer(w, file); err != nil {
		fmt.Printf("传输图片失败: %v\n", err)
	}
}
