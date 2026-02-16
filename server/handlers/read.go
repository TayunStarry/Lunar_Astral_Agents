package handlers

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"io"                                // 导入io包，用于文件操作
	"log"                               // 导入log包，用于日志记录
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"os"                                // 导入os包，用于文件操作
	"path/filepath"                     // 导入path/filepath包，用于文件路径操作
	"strconv"                           // 导入strconv包，用于字符串和其他类型的转换
	"strings"                           // 导入strings包，用于字符串操作
)

// ReadHandler 处理文件读取请求，从本地目录读取文件并返回给客户端
func ReadHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为 GET，如果不是则返回 405 Method Not Allowed 错误
	if r.Method != "GET" {
		http.Error(w, "Read请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求 URL 路径中去除 "/read/" 前缀，获取实际的文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/read/")
	// 检查文件路径是否为空，如果为空则返回 400 Bad Request 错误
	if filePath == "" {
		http.Error(w, "Read请求[ERROR] -> 未指定文件路径", http.StatusBadRequest)
		return
	}
	// 拼接配置中的本地目录和请求的文件路径，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(config.LocalDir, filePath))
	// 检查拼接后的路径是否在配置的本地目录下，如果不是则返回 403 Forbidden 错误
	if !strings.HasPrefix(fullPath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "Read请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}
	// 获取文件信息与错误内容
	fileInfo, err := os.Stat(fullPath)
	// 检查文件是否不存在，如果不存在则返回 404 Not Found 错误
	if os.IsNotExist(err) {
		http.Error(w, "Read请求[ERROR] -> 文件未找到", http.StatusNotFound)
		return
	}
	// 检查获取文件信息是否出错，如果出错则返回 500 Internal Server Error 错误
	if err != nil {
		http.Error(w, "Read请求[ERROR] -> 获取文件信息失败", http.StatusInternalServerError)
		return
	}
	// 检查路径是否为目录，如果是则返回 400 Bad Request 错误
	if fileInfo.IsDir() {
		http.Error(w, "Read请求[ERROR] -> 不允许读取目录", http.StatusBadRequest)
		return
	}
	// 获取文件扩展名并转换为小写
	ext := strings.ToLower(filepath.Ext(fullPath))
	// 根据文件扩展名设置 Content-Type 头信息
	if mimeType, ok := config.MimeMap[ext]; ok {
		w.Header().Set("Content-Type", mimeType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	// 设置 Content-Length 头信息，值为文件大小
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	// 打开文件
	file, err := os.Open(fullPath)
	// 检查打开文件是否出错，如果出错则返回 500 Internal Server Error 错误
	if err != nil {
		http.Error(w, "Read请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
		return
	}
	// 关闭文件，确保在函数返回前关闭文件句柄
	defer file.Close()
	// 将文件内容复制到响应中返回给客户端
	io.Copy(w, file)
	// 记录读取成功日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Read请求 -> 成功读取: %s, 大小: %d 字节", fullPath, fileInfo.Size())
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
