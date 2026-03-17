package handlers

// 导入所需的包
import (
	execute "Lunar-Astral-Agents/files" // 导入执行模块，用于处理核心逻辑
	"encoding/json"                     // 导入json包，用于解析请求体
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"strings"                           // 导入strings包，用于字符串操作
)

// FileListHandler 处理文件列表请求，返回指定目录下的文件和子目录信息
func FileListHandler(w http.ResponseWriter, r *http.Request) {
	// 从请求 URL 路径中去除 "/file_list/" 前缀
	path := strings.TrimPrefix(r.URL.Path, "/file_list/")
	// 调用 execute 模块获取文件列表
	fileList, err := execute.GetFileList(path)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "访问被拒绝":
			http.Error(w, "FileList请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		case "目录未找到":
			http.Error(w, "FileList请求[ERROR] -> 目录未找到", http.StatusNotFound)
		case "不是一个目录":
			http.Error(w, "FileList请求[ERROR] -> 不是一个目录", http.StatusBadRequest)
		case "读取目录失败":
			http.Error(w, "FileList请求[ERROR] -> 读取目录失败", http.StatusInternalServerError)
		default:
			http.Error(w, "FileList请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 设置响应的 Content-Type 为 JSON 格式
	w.Header().Set("Content-Type", "application/json")
	// 将文件列表以 JSON 格式编码并写入响应
	json.NewEncoder(w).Encode(fileList)
}
