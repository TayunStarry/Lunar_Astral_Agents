package file

import (
	"LunarCore/FileSystem"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ArchiveHandler 处理 ZIP 压缩和解压请求
func ArchiveHandler(w http.ResponseWriter, r *http.Request) {
	// 根据请求方法进行分支处理
	switch r.Method {

	case "POST":
		// 处理创建 ZIP 文件的请求
		createZip(w, r)

	case "PUT":
		// 处理解压 ZIP 文件的请求
		extractZip(w, r)

	default:
		// 处理不允许的请求方法，返回错误响应
		http.Error(w, "Archive请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
	}
}

// createZip 创建 ZIP 压缩文件并直接返回给客户端
func createZip(w http.ResponseWriter, r *http.Request) {
	// 解析多部分表单数据，设置最大内存为 32MB
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		// 若解析失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 解析表单失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 从表单中获取名为 "files" 的文件列表
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		// 若未选择文件，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 未选择文件", http.StatusBadRequest)
		return
	}
	// 从表单中获取 ZIP 文件名，若未提供则使用默认名 "archive.zip"
	zipName := r.FormValue("zip_name")
	if zipName == "" {
		zipName = "archive.zip"
	}
	// 调用 execute 模块创建 ZIP 文件
	zipData, err := FileSystem.CreateZip(files, zipName)
	if err != nil {
		// 若创建失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 确保 ZIP 文件名以 ".zip" 结尾
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	// 设置响应头，指定响应内容类型为 ZIP 文件
	w.Header().Set("Content-Type", "application/zip")
	// 设置响应头，指定文件下载时的文件名
	w.Header().Set("Content-Disposition", "attachment; filename="+zipName)
	// 设置响应头，指定响应内容的长度
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))
	// 将 ZIP 文件数据写入响应
	_, err = w.Write(zipData)
	if err != nil {
		// 若发送文件失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 发送ZIP文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// extractZip 解压 ZIP 文件并返回文件列表给客户端
func extractZip(w http.ResponseWriter, r *http.Request) {
	// 解析多部分表单，设置最大内存为 32MB
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		// 若解析失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 解析表单失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 从表单中获取名为 "zip_file" 的 ZIP 文件
	file, header, err := r.FormFile("zip_file")
	if err != nil {
		// 若获取文件失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 获取ZIP文件失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 函数结束时关闭文件，防止资源泄漏
	defer file.Close()
	// 调用 execute 模块解压 ZIP 文件
	extractedFiles, _, err := FileSystem.ExtractZip(file)
	if err != nil {
		// 若解压失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 设置响应头，指定响应内容类型为 JSON
	w.Header().Set("Content-Type", "application/json")
	// 设置响应状态码为 200 OK
	w.WriteHeader(http.StatusOK)
	// 构建响应数据
	response := map[string]any{
		"total_files":     len(extractedFiles), // 解压出的文件总数
		"extracted_files": extractedFiles,      // 解压出的文件信息列表
		"original_zip":    header.Filename,     // 原始 ZIP 文件名
	}
	// 将响应数据编码为 JSON 并写入响应
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// 若编码失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 生成响应失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
