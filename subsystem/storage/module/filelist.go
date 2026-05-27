package module

import (
	"config"
	"fmt"
	"logger"
	"os"
	"path/filepath"
	"strings"
)

// GetFileList 获取指定目录下的文件和子目录信息
func GetFileList(path string) ([]FileInfo, error) {
	// 如果路径为空，则默认使用当前目录
	if path == "" {
		path = "."
	}
	// 拼接完整路径并清理路径格式
	fullPath := filepath.Clean(filepath.Join(*config.LocalDir, path))
	// 检查请求路径是否在允许的目录范围内，防止路径遍历攻击
	if !strings.HasPrefix(fullPath, filepath.Clean(*config.LocalDir)) {
		return nil, fmt.Errorf("访问被拒绝")
	}
	// 获取文件或目录的信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("目录未找到")
	}
	// 检查路径是否为目录
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("不是一个目录")
	}
	// 读取目录下的所有文件和子目录
	files, err := os.ReadDir(fullPath)
	// 若读取目录失败，返回错误
	if err != nil {
		return nil, fmt.Errorf("读取目录失败")
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
		relPath, err := filepath.Rel(*config.LocalDir, filepath.Join(fullPath, file.Name()))
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
	logger.SubInfo("Storage", "FileList", "成功获取目录: %s, 包含 %d 个条目", fullPath, len(fileList))
	return fileList, nil
}
