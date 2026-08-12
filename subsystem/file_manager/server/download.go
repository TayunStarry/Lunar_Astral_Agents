package server

import (
	"LunarSubsystem/FileManager/module"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// DownloadHandler 处理文件下载请求
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	// 从请求 URL 路径中去除 "/file/download/" 前缀，获取实际文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/file/download/")

	// 执行下载操作
	fullPath, fileSize, err := module.GetFileInfo(filePath)
	if err != nil {
		switch err.Error() {
		case "未指定文件":
			http.Error(w, "Download请求[ERROR] -> 未指定文件", http.StatusBadRequest)
		case "访问被拒绝":
			http.Error(w, "Download请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		case "文件未找到":
			http.Error(w, "Download请求[ERROR] -> 文件未找到", http.StatusNotFound)
		case "无法下载目录":
			http.Error(w, "Download请求[ERROR] -> 无法下载目录", http.StatusBadRequest)
		default:
			http.Error(w, "Download请求[ERROR] -> 下载失败: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// 设置响应头，指定文件以附件形式下载，并设置文件名
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filePath))

	// 设置响应头，指定文件类型为二进制流
	w.Header().Set("Content-Type", "application/octet-stream")

	// 设置响应头，指定文件大小
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))

	// 将文件内容发送给客户端
	http.ServeFile(w, r, fullPath)
}
