package server

import (
	"storage/module"
	"encoding/json"
	"net/http"
	"strings"
)

// DeleteHandler 处理 DELETE 请求，用于删除指定路径下的文件或目录
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为 DELETE，如果不是则返回方法不允许的错误
	if r.Method != "DELETE" {
		http.Error(w, "Delete请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求 URL 路径中去除 "/delete/" 前缀，获取要删除的文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/delete/")
	// 调用 execute 模块删除文件
	fullPath, err := module.DeleteFile(filePath)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "未指定文件":
			http.Error(w, "Delete请求[ERROR] -> 未指定文件", http.StatusBadRequest)
		case "访问被拒绝":
			http.Error(w, "Delete请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		case "文件未找到":
			http.Error(w, "Delete请求[ERROR] -> 文件未找到", http.StatusNotFound)
		case "删除文件失败":
			http.Error(w, "Delete请求[ERROR] -> 删除文件失败", http.StatusInternalServerError)
		default:
			http.Error(w, "Delete请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 设置响应状态码为 200 OK
	w.WriteHeader(http.StatusOK)
	// 将删除成功的消息和文件路径以 JSON 格式返回给客户端
	json.NewEncoder(w).Encode(
		map[string]string{
			"path": fullPath,
		},
	)
}
