package module

import (
	"config"
	"fmt"
	"logger"
	"os"
	"path/filepath"
	"strings"
)

// GetFileInfo 获取文件信息
func GetFileInfo(filePath string) (string, int64, error) {
	// 检查文件路径是否为空
	if filePath == "" {
		return "", 0, fmt.Errorf("未指定文件")
	}

	// 拼接配置中的本地目录和请求的文件路径，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(*config.LocalDir, filePath))

	// 检查拼接后的完整路径是否在配置的本地目录下，防止路径遍历攻击
	if !strings.HasPrefix(fullPath, filepath.Clean(*config.LocalDir)) {
		return "", 0, fmt.Errorf("访问被拒绝")
	}

	// 获取文件信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, fmt.Errorf("文件未找到")
		}
		return "", 0, err
	}

	// 检查路径是否为目录
	if fileInfo.IsDir() {
		return "", 0, fmt.Errorf("无法下载目录")
	}

	logger.Info("Storage", "Download请求 -> 成功下载: %s, 大小: %d 字节", fullPath, fileInfo.Size())

	return fullPath, fileInfo.Size(), nil
}
