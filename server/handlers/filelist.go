package handlers

// 导入所需的包
import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"encoding/json"                     // 导入json包，用于解析请求体
	"log"                               // 导入log包，用于日志记录
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"os"                                // 导入os包，用于文件操作
	"path/filepath"                     // 导入path/filepath包，用于文件路径操作
	"strings"                           // 导入strings包，用于字符串操作
	"time"                              // 导入time包，用于时间操作
)

// FileListHandler 处理文件列表请求，返回指定目录下的文件和子目录信息
func FileListHandler(w http.ResponseWriter, r *http.Request) {
	// 从请求 URL 路径中去除 "/file_list/" 前缀
	path := strings.TrimPrefix(r.URL.Path, "/file_list/")
	// 如果路径为空，则默认使用当前目录
	if path == "" {
		path = "."
	}
	// 拼接完整路径并清理路径格式
	fullPath := filepath.Clean(filepath.Join(config.LocalDir, path))
	// 检查请求路径是否在允许的目录范围内，防止路径遍历攻击
	if !strings.HasPrefix(fullPath, filepath.Clean(config.LocalDir)) {
		// 若不在允许范围内，返回 403 禁止访问错误
		http.Error(w, "FileList请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}
	// 获取文件或目录的信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		// 若获取信息失败，返回 404 未找到错误
		http.Error(w, "FileList请求[ERROR] -> 目录未找到", http.StatusNotFound)
		return
	}
	// 检查路径是否为目录
	if !fileInfo.IsDir() {
		// 若不是目录，返回 400 错误请求错误
		http.Error(w, "FileList请求[ERROR] -> 不是一个目录", http.StatusBadRequest)
		return
	}
	// 读取目录下的所有文件和子目录
	files, err := os.ReadDir(fullPath)
	// 若读取目录失败，返回 500 内部服务器错误
	if err != nil {
		http.Error(w, "FileList请求[ERROR] -> 读取目录失败", http.StatusInternalServerError)
		return
	}
	// 定义文件信息结构体，用于存储文件和目录的相关信息
	type FileInfo struct {
		Name         string    `json:"name"`         // 文件名或目录名
		Size         int64     `json:"size"`         // 文件大小
		IsDir        bool      `json:"isDir"`        // 是否为目录
		LastModified time.Time `json:"lastModified"` // 最后修改时间
		Path         string    `json:"path"`         // 相对于配置目录的相对路径
	}
	// 初始化文件信息列表
	var fileList []FileInfo
	// 遍历目录下的所有文件和子目录
	for _, file := range files {
		// 获取文件或目录的详细信息
		info, err := file.Info()
		// 若获取信息失败，跳过当前文件
		if err != nil {
			continue
		}
		// 计算文件或目录相对于配置目录的相对路径
		relPath, err := filepath.Rel(config.LocalDir, filepath.Join(fullPath, file.Name()))
		// 若计算失败，使用文件名作为相对路径
		if err != nil {
			relPath = file.Name()
		}
		// 将文件信息添加到文件列表中
		fileList = append(fileList, FileInfo{
			Name:         file.Name(),
			Size:         info.Size(),
			IsDir:        file.IsDir(),
			LastModified: info.ModTime(),
			Path:         relPath,
		})
	}
	// 设置响应的 Content-Type 为 JSON 格式
	w.Header().Set("Content-Type", "application/json")
	// 将文件列表以 JSON 格式编码并写入响应
	json.NewEncoder(w).Encode(fileList)
	// 记录文件列表请求日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("FileList请求 -> 成功获取目录: %s, 包含 %d 个条目", fullPath, len(fileList))
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
