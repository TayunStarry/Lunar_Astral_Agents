package hierarchy

import (
	"LunarCore/config"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DeleteFile 删除指定路径的文件或目录
func DeleteFile(filePath string) (string, error) {
	// 检查文件路径是否为空
	if filePath == "" {
		return "", fmt.Errorf("未指定文件")
	}
	// 将配置中的本地目录和请求的文件路径拼接，并清理路径格式
	fullPath := filepath.Clean(filepath.Join(*config.LocalDir, filePath))
	// 检查拼接后的完整路径是否在配置的本地目录下
	if !strings.HasPrefix(fullPath, filepath.Clean(*config.LocalDir)) {
		return "", fmt.Errorf("访问被拒绝")
	}
	// 检查文件或目录是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("文件未找到")
	}
	// 获取文件锁，用于保证文件操作的原子性
	lock := GetFileLock(fullPath)
	// 加锁，防止并发操作
	lock.Lock()
	// 函数返回时自动解锁
	defer lock.Unlock()
	// 尝试删除文件或目录
	if err := os.RemoveAll(fullPath); err != nil {
		return "", fmt.Errorf("删除文件失败: %w", err)
	}
	// 从文件锁映射中删除该文件的锁
	FileLocks.Delete(fullPath)
	// 记录删除成功日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Delete请求 -> 成功删除: %s", fullPath)
		log.Printf("%s", strings.Repeat("-=", 28))
	}
	return fullPath, nil
}
