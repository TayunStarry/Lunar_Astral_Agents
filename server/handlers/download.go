package handlers

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"fmt"                               // 导入fmt包，用于格式化输出
	"log"                               // 导入log包，用于日志记录
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"os"                                // 导入os包，用于文件操作
	"path/filepath"                     // 导入path/filepath包，用于文件路径操作
	"strings"                           // 导入strings包，用于字符串操作
)

// DownloadHandler 处理文件下载请求
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	// 从请求 URL 路径中去除 "/download/" 前缀，获取实际文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/download/")
	// 检查文件路径是否为空，如果为空则返回错误响应
	if filePath == "" {
		http.Error(w, "Download请求[ERROR] -> 未指定文件", http.StatusBadRequest)
		return
	}
	// 拼接配置中的本地目录和请求的文件路径，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(config.LocalDir, filePath))
	// 检查拼接后的完整路径是否在配置的本地目录下，防止路径遍历攻击
	if !strings.HasPrefix(fullPath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "Download请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}
	// 获取文件信息
	fileInfo, err := os.Stat(fullPath)
	// 检查文件是否不存在，如果不存在则返回错误响应
	if os.IsNotExist(err) {
		http.Error(w, "Download请求[ERROR] -> 文件未找到", http.StatusNotFound)
		return
	}
	// 检查路径是否为目录，如果是目录则返回错误响应
	if fileInfo.IsDir() {
		http.Error(w, "Download请求[ERROR] -> 无法下载目录", http.StatusBadRequest)
		return
	}
	// 设置响应头，指定文件以附件形式下载，并设置文件名
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
	// 设置响应头，指定文件类型为二进制流
	w.Header().Set("Content-Type", "application/octet-stream")
	// 设置响应头，指定文件大小
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	// 将文件内容发送给客户端
	http.ServeFile(w, r, fullPath)
	// 记录下载成功日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Download请求 -> 成功下载: %s, 大小: %d 字节", fullPath, fileInfo.Size())
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
