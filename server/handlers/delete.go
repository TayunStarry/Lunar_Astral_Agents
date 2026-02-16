package handlers

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"encoding/json"                     // 导入json包，用于解析请求体
	"log"                               // 导入log包，用于日志记录
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"os"                                // 导入os包，用于文件操作
	"path/filepath"                     // 导入path/filepath包，用于文件路径操作
	"strings"                           // 导入strings包，用于字符串操作
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
	// 检查文件路径是否为空，如果为空则返回未指定文件的错误
	if filePath == "" {
		http.Error(w, "Delete请求[ERROR] -> 未指定文件", http.StatusBadRequest)
		return
	}
	// 将配置中的本地目录和请求的文件路径拼接，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(config.LocalDir, filePath))
	// 检查拼接后的完整路径是否在配置的本地目录下，如果不是则返回访问被拒绝的错误
	if !strings.HasPrefix(fullPath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "Delete请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}
	// 检查文件或目录是否存在，如果不存在则返回文件未找到的错误
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "Delete请求[ERROR] -> 文件未找到", http.StatusNotFound)
		return
	}
	// 获取文件锁，用于保证文件操作的原子性
	fileLock := getFileLock(fullPath)
	// 加锁，防止并发操作
	fileLock.Lock()
	// 函数返回时自动解锁
	defer fileLock.Unlock()
	// 尝试删除文件或目录，如果删除失败则返回删除文件失败的错误
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, "Delete请求[ERROR] -> 删除文件失败", http.StatusInternalServerError)
		return
	}
	// 从文件锁映射中删除该文件的锁
	FileLocks.Delete(fullPath)
	// 设置响应状态码为 200 OK
	w.WriteHeader(http.StatusOK)
	// 将删除成功的消息和文件路径以 JSON 格式返回给客户端
	json.NewEncoder(w).Encode(
		map[string]string{
			"path": fullPath,
		},
	)
	// 记录删除成功日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Delete请求 -> 成功删除: %s", fullPath)
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
