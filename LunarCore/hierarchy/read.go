package hierarchy

import (
	"LunarCore/config"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile 读取指定路径的文件内容
func ReadFile(filePath string) (io.ReadCloser, int64, string, error) {
	// 检查文件路径是否为空
	if filePath == "" {
		return nil, 0, "", fmt.Errorf("未指定文件路径")
	}
	// 拼接配置中的本地目录和请求的文件路径，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(*config.LocalDir, filePath))
	// 检查拼接后的路径是否在配置的本地目录下
	if !strings.HasPrefix(fullPath, filepath.Clean(*config.LocalDir)) {
		return nil, 0, "", fmt.Errorf("访问被拒绝")
	}
	// 获取文件信息与错误内容
	fileInfo, err := os.Stat(fullPath)
	// 检查文件是否不存在
	if os.IsNotExist(err) {
		return nil, 0, "", fmt.Errorf("文件未找到")
	}
	// 检查获取文件信息是否出错
	if err != nil {
		return nil, 0, "", fmt.Errorf("获取文件信息失败")
	}
	// 检查路径是否为目录
	if fileInfo.IsDir() {
		return nil, 0, "", fmt.Errorf("不允许读取目录")
	}
	// 获取文件扩展名并转换为小写
	ext := strings.ToLower(filepath.Ext(fullPath))
	// 根据文件扩展名设置 Content-Type
	mimeType := "application/octet-stream"
	if mt, ok := config.MimeMap[ext]; ok {
		mimeType = mt
	}
	// 打开文件
	file, err := os.Open(fullPath)
	// 检查打开文件是否出错
	if err != nil {
		return nil, 0, "", fmt.Errorf("打开文件失败")
	}
	// 记录读取成功日志
	if *config.Developer {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Read请求 -> 成功读取: %s, 大小: %d 字节", fullPath, fileInfo.Size())
		log.Printf("%s", strings.Repeat("-=", 28))
	}
	return file, fileInfo.Size(), mimeType, nil
}
