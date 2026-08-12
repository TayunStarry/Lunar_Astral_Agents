package server

import (
	"LunarSubsystem/FileManager/module"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ReadHandler 处理文件读取请求，从本地目录读取文件并返回给客户端
func ReadHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为 GET，如果不是则返回 405 Method Not Allowed 错误
	if r.Method != "GET" {
		http.Error(w, "Read请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求 URL 路径中去除 "/file/read/" 前缀，获取实际的文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/file/read/")
	// 调用 execute 模块读取文件
	file, size, mimeType, err := module.ReadFile(filePath)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "未指定文件路径":
			http.Error(w, "Read请求[ERROR] -> 未指定文件路径", http.StatusBadRequest)
		case "访问被拒绝":
			http.Error(w, "Read请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		case "文件未找到":
			http.Error(w, "Read请求[ERROR] -> 文件未找到", http.StatusNotFound)
		case "获取文件信息失败":
			http.Error(w, "Read请求[ERROR] -> 获取文件信息失败", http.StatusInternalServerError)
		case "不允许读取目录":
			http.Error(w, "Read请求[ERROR] -> 不允许读取目录", http.StatusBadRequest)
		case "打开文件失败":
			http.Error(w, "Read请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
		default:
			http.Error(w, "Read请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 确保文件在使用后关闭
	defer file.Close()
	// 设置 Content-Type 头信息
	w.Header().Set("Content-Type", mimeType)
	// 设置 Content-Length 头信息，值为文件大小
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	// 将文件内容复制到响应中返回给客户端
	io.Copy(w, file)
}
