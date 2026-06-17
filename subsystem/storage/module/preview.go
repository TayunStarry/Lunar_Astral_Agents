package module

import (
	"fmt"
	"io"
	"logger"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// 允许预览的文件类型白名单
var previewAllowlist = map[string]bool{
	// 图片格式
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".svg":  true,
	".ico":  true,
	".bmp":  true,
	".tiff": true,
	".tif":  true,
	// 视频格式
	".mp4":  true,
	".webm": true,
	".avi":  true,
	".mov":  true,
	".mkv":  true,
	".wmv":  true,
	".flv":  true,
	".m4v":  true,
	".mpg":  true,
	".mpeg": true,
	// 文本格式（仅数据与配置文件，不含代码/脚本）
	".txt":  true,
	".md":   true,
	".log":  true,
	".csv":  true,
	".json": true,
	".xml":  true,
	".yaml": true,
	".yml":  true,
	".toml": true,
	".ini":  true,
	".cfg":  true,
}

// 图片扩展名集合（用于快速判断是否为图片类型）
var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
	".gif": true, ".svg": true, ".ico": true, ".bmp": true,
	".tiff": true, ".tif": true,
}

// 视频扩展名集合
var videoExtensions = map[string]bool{
	".mp4": true, ".webm": true, ".avi": true, ".mov": true,
	".mkv": true, ".wmv": true, ".flv": true, ".m4v": true,
	".mpg": true, ".mpeg": true,
}

// PreviewFile 预览指定绝对路径的文件内容，不受本地目录限制
// 仅允许读取图片、视频、文本三种类型文件
func PreviewFile(filePath string) (io.ReadCloser, int64, string, string, error) {
	// 1. 检查路径是否为空
	if filePath == "" {
		return nil, 0, "", "", fmt.Errorf("未指定文件路径")
	}

	// 2. 清理路径并检测路径遍历攻击
	cleanPath := filepath.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		// 在已清理的路径中再次检测，防止编码绕过
		logger.SubWarn("Storage", "Preview", "检测到路径遍历尝试: %s", filePath)
		return nil, 0, "", "", fmt.Errorf("路径包含非法字符")
	}

	// 3. 获取文件信息，检查是否为目录
	fileInfo, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		return nil, 0, "", "", fmt.Errorf("文件未找到")
	}
	if err != nil {
		logger.SubError("Storage", "Preview", "获取文件信息失败: %s, %v", cleanPath, err)
		return nil, 0, "", "", fmt.Errorf("获取文件信息失败")
	}

	// 4. 拒绝目录
	if fileInfo.IsDir() {
		return nil, 0, "", "", fmt.Errorf("不允许读取目录")
	}

	// 5. 文件大小限制（最大 500MB，防止内存溢出）
	const maxFileSize int64 = 500 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		logger.SubWarn("Storage", "Preview", "文件过大被拒绝: %s, 大小: %d", cleanPath, fileInfo.Size())
		return nil, 0, "", "", fmt.Errorf("文件大小超过限制 (最大 500MB)")
	}

	// 6. 检查文件扩展名是否在白名单中
	ext := strings.ToLower(filepath.Ext(cleanPath))
	if !previewAllowlist[ext] {
		logger.SubWarn("Storage", "Preview", "文件类型不被允许: %s (扩展名: %s)", cleanPath, ext)
		return nil, 0, "", "", fmt.Errorf("不允许的文件类型: %s", ext)
	}

	// 7. 确定文件类别
	category := "text"
	if imageExtensions[ext] {
		category = "image"
	} else if videoExtensions[ext] {
		category = "video"
	}

	// 8. 获取 MIME 类型
	mimeType := "application/octet-stream"
	if mt, ok := mimeTypeByExt(ext); ok {
		mimeType = mt
	} else if mt := mime.TypeByExtension(ext); mt != "" {
		mimeType = mt
	}

	// 9. 打开文件
	file, err := os.Open(cleanPath)
	if err != nil {
		logger.SubError("Storage", "Preview", "打开文件失败: %s, %v", cleanPath, err)
		return nil, 0, "", "", fmt.Errorf("打开文件失败")
	}

	logger.SubInfo("Storage", "Preview", "预览成功: %s, 类别: %s, 大小: %d 字节", cleanPath, category, fileInfo.Size())
	return file, fileInfo.Size(), mimeType, category, nil
}

// mimeTypeByExt 根据扩展名返回对应的 MIME 类型
func mimeTypeByExt(ext string) (string, bool) {
	mimeMap := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".bmp":  "image/bmp",
		".tiff": "image/tiff",
		".tif":  "image/tiff",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".mkv":  "video/x-matroska",
		".txt":  "text/plain",
		".md":   "text/markdown",
		".json": "application/json",
		".xml":  "application/xml",
		".yaml": "text/yaml",
		".yml":  "text/yaml",
		".toml": "text/toml",
		".ini":  "text/plain",
		".cfg":  "text/plain",
		".log":  "text/plain",
		".csv":  "text/csv",
	}
	mt, ok := mimeMap[ext]
	return mt, ok
}
