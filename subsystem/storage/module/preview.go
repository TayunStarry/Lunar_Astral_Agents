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

// 文件类别常量
const (
	CategoryImage = "image"
	CategoryVideo = "video"
	CategoryText  = "text"
)

// PreviewEntry 文件预览条目，包含 MIME 类型和文件类别
type PreviewEntry struct {
	MIME     string // MIME 类型
	Category string // 文件类别: image / video / text
}

// 允许预览的文件类型白名单（含分类与 MIME 信息，三合一）
var previewAllowlist = map[string]PreviewEntry{
	// 图片格式
	".png":  {MIME: "image/png", Category: CategoryImage},
	".jpg":  {MIME: "image/jpeg", Category: CategoryImage},
	".jpeg": {MIME: "image/jpeg", Category: CategoryImage},
	".webp": {MIME: "image/webp", Category: CategoryImage},
	".gif":  {MIME: "image/gif", Category: CategoryImage},
	".svg":  {MIME: "image/svg+xml", Category: CategoryImage},
	".ico":  {MIME: "image/x-icon", Category: CategoryImage},
	".bmp":  {MIME: "image/bmp", Category: CategoryImage},
	".tiff": {MIME: "image/tiff", Category: CategoryImage},
	".tif":  {MIME: "image/tiff", Category: CategoryImage},
	// 视频格式
	".mp4":  {MIME: "video/mp4", Category: CategoryVideo},
	".webm": {MIME: "video/webm", Category: CategoryVideo},
	".avi":  {MIME: "video/x-msvideo", Category: CategoryVideo},
	".mov":  {MIME: "video/quicktime", Category: CategoryVideo},
	".mkv":  {MIME: "video/x-matroska", Category: CategoryVideo},
	".wmv":  {MIME: "video/x-ms-wmv", Category: CategoryVideo},
	".flv":  {MIME: "video/x-flv", Category: CategoryVideo},
	".m4v":  {MIME: "video/mp4", Category: CategoryVideo},
	".mpg":  {MIME: "video/mpeg", Category: CategoryVideo},
	".mpeg": {MIME: "video/mpeg", Category: CategoryVideo},
	// 文本格式（仅数据与配置文件，不含代码/脚本）
	".txt":  {MIME: "text/plain", Category: CategoryText},
	".md":   {MIME: "text/markdown", Category: CategoryText},
	".log":  {MIME: "text/plain", Category: CategoryText},
	".csv":  {MIME: "text/csv", Category: CategoryText},
	".json": {MIME: "application/json", Category: CategoryText},
	".xml":  {MIME: "application/xml", Category: CategoryText},
	".yaml": {MIME: "text/yaml", Category: CategoryText},
	".yml":  {MIME: "text/yaml", Category: CategoryText},
	".toml": {MIME: "text/toml", Category: CategoryText},
	".ini":  {MIME: "text/plain", Category: CategoryText},
	".cfg":  {MIME: "text/plain", Category: CategoryText},
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

	// 6. 检查文件扩展名是否在白名单中，同时获取 MIME 和类别
	ext := strings.ToLower(filepath.Ext(cleanPath))
	entry, ok := previewAllowlist[ext]
	if !ok {
		logger.SubWarn("Storage", "Preview", "文件类型不被允许: %s (扩展名: %s)", cleanPath, ext)
		return nil, 0, "", "", fmt.Errorf("不允许的文件类型: %s", ext)
	}

	// 7. 获取 MIME 类型（白名单优先，标准库兜底）
	mimeType := entry.MIME
	if mimeType == "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			mimeType = mt
		} else {
			mimeType = "application/octet-stream"
		}
	}

	// 8. 打开文件
	file, err := os.Open(cleanPath)
	if err != nil {
		logger.SubError("Storage", "Preview", "打开文件失败: %s, %v", cleanPath, err)
		return nil, 0, "", "", fmt.Errorf("打开文件失败")
	}

	logger.SubInfo("Storage", "Preview", "预览成功: %s, 类别: %s, 大小: %d 字节", cleanPath, entry.Category, fileInfo.Size())
	return file, fileInfo.Size(), mimeType, entry.Category, nil
}
