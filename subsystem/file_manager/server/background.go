package server

import (
	"LunarSubsystem/file_manager/module"
	"LunarSubsystem/general_config"
	"LunarSubsystem/general_logger"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// fallbackImageURL 本地背景图不可用时使用的网络图片
const fallbackImageURL = "https://picsum.photos/1920/1080"

// RandomBackgroundHandler 服务随机选择的背景图片，本地无图片时回退到网络图片
func RandomBackgroundHandler(w http.ResponseWriter, _ *http.Request) {
	filename, err := module.GetRandomBackgroundImage()
	if err != nil {
		logger.Warn("Storage", "获取随机背景图失败: %v，回退到网络图片", err)
		serveNetworkImage(w)
		return
	}

	backgroundDir := filepath.Join(*config.LocalDir, "images/background")
	filePath := filepath.Join(backgroundDir, filename)
	file, err := os.Open(filePath)
	if err != nil {
		logger.Warn("Storage", "打开背景图文件失败: %v，回退到网络图片", err)
		serveNetworkImage(w)
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
		logger.Error("Storage", "传输图片失败: %v", err)
	}
}

// serveNetworkImage 从网络获取图片并返回给客户端
func serveNetworkImage(w http.ResponseWriter) {
	resp, err := http.Get(fallbackImageURL)
	if err != nil {
		logger.Error("Storage", "获取网络图片失败: %v", err)
		http.Error(w, "无法获取背景图片", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, resp.Body); err != nil {
		logger.Error("Storage", "传输网络图片失败: %v", err)
	}
}
