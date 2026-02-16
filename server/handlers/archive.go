package handlers

import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"archive/zip"                       // 用于 ZIP 压缩和解压
	"bytes"                             // 用于在内存中创建 ZIP 文件
	"encoding/json"                     // 用于处理 JSON 数据
	"fmt"                               // 用于格式化字符串
	"io"                                // 用于处理输入输出操作
	"log"                               // 用于日志记录
	"net/http"                          // 用于处理 HTTP 请求和响应
	"path/filepath"                     // 用于处理文件路径
	"strings"                           // 用于字符串操作
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
	// 确保 ZIP 文件名以 ".zip" 结尾
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	// 创建一个内存缓冲区，用于存储 ZIP 文件内容
	var zipBuffer bytes.Buffer
	// 创建一个新的 ZIP 写入器，将内容写入缓冲区
	zipWriter := zip.NewWriter(&zipBuffer)
	// 遍历所有要压缩的文件
	for _, fileHeader := range files {
		// 打开表单中的文件
		file, openErr := fileHeader.Open()
		if openErr != nil {
			// 若打开文件失败，返回错误响应给客户端
			http.Error(w, "Archive请求[ERROR] -> 打开文件失败: "+openErr.Error(), http.StatusInternalServerError)
			return
		}
		// 创建 ZIP 文件头，设置文件名和压缩方法为 Deflate
		zipHeader := &zip.FileHeader{
			Name:   fileHeader.Filename,
			Method: zip.Deflate,
		}
		// 在 ZIP 文件中创建一个新的条目
		zipFileWriter, createErr := zipWriter.CreateHeader(zipHeader)
		if createErr != nil {
			// 若创建条目失败，关闭已打开的文件并返回错误响应给客户端
			file.Close()
			http.Error(w, "Archive请求[ERROR] -> 创建ZIP条目失败: "+createErr.Error(), http.StatusInternalServerError)
			return
		}
		// 将文件内容复制到 ZIP 条目中
		_, err = io.Copy(zipFileWriter, file)
		// 关闭已打开的文件，释放资源
		file.Close()
		if err != nil {
			// 若写入内容失败，返回错误响应给客户端
			http.Error(w, "Archive请求[ERROR] -> 写入ZIP内容失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// 关闭 ZIP 写入器，完成 ZIP 文件的创建
	err = zipWriter.Close()
	if err != nil {
		// 若关闭写入器失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 关闭ZIP写入器失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 从缓冲区获取 ZIP 文件的字节数据
	zipData := zipBuffer.Bytes()
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
	// 记录日志，包含创建的 ZIP 文件名和包含的文件数量
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Archive请求 -> 成功创建ZIP文件: %s, 包含 %d 个文件", zipName, len(files))
		log.Printf("%s", strings.Repeat("-=", 28))
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
	// 在内存中读取 ZIP 文件的全部内容
	zipData, err := io.ReadAll(file)
	if err != nil {
		// 若读取失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 读取ZIP文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 从字节数据创建一个 ZIP 读取器，用于后续解压操作
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		// 若创建读取器失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 打开ZIP文件失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 用于存储解压后的文件信息
	var extractedFiles []map[string]any
	// 遍历 ZIP 读取器中的所有文件
	for _, zipFile := range zipReader.File {
		// 跳过目录，只处理文件
		if zipFile.FileInfo().IsDir() {
			continue
		}
		// 打开 ZIP 中的文件，获取一个可读的文件句柄
		rc, err := zipFile.Open()
		if err != nil {
			// 若打开文件失败，返回错误响应给客户端
			http.Error(w, "Archive请求[ERROR] -> 打开ZIP内文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// 读取 ZIP 中文件的全部内容
		fileContent, err := io.ReadAll(rc)
		// 关闭文件句柄，释放资源
		rc.Close()
		if err != nil {
			// 若读取内容失败，返回错误响应给客户端
			http.Error(w, "Archive请求[ERROR] -> 读取ZIP内文件内容失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// 获取文件的基本信息
		fileInfo := zipFile.FileInfo()
		// 将解压后的文件信息添加到列表中
		extractedFiles = append(extractedFiles, map[string]any{
			"name":          zipFile.Name,                                // 文件名
			"size":          fileInfo.Size(),                             // 文件大小
			"content":       fileContent,                                 // 文件内容
			"last_modified": fileInfo.ModTime(),                          // 最后修改时间
			"extension":     strings.ToLower(filepath.Ext(zipFile.Name)), // 文件扩展名
		})
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
	// 记录日志，包含原始 ZIP 文件名和解压出的文件数量
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Archive请求 -> 成功解压ZIP文件: %s, 解压出 %d 个文件", header.Filename, len(extractedFiles))
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
