package module

import (
	"config"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SaveFile 保存文件到指定路径
func SaveFile(fileName string, overwrite bool, body io.Reader) (string, string, error) {
	// 检查文件名是否为空
	if fileName == "" {
		return "", "", fmt.Errorf("缺少文件名")
	}
	// 检查文件名是否为 "." 或 ".."，防止目录遍历攻击
	if fileName == "." || fileName == ".." {
		return "", "", fmt.Errorf("无效的文件名")
	}
	// 拼接文件的完整路径
	fullPath := filepath.Join(*config.LocalDir, fileName)
	// 创建文件所在的目录
	if mkdirErr := os.MkdirAll(filepath.Dir(fullPath), 0755); mkdirErr != nil {
		return "", "", fmt.Errorf("创建目录失败: %w", mkdirErr)
	}
	// 获取文件锁并加锁，确保文件操作的原子性
	lock := GetFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()
	// 检查文件是否存在
	_, err := os.Stat(fullPath)
	// 如果获取文件状态时发生了其他错误
	if err != nil && !os.IsNotExist(err) {
		return "", "", fmt.Errorf("获取文件状态失败: %w", err)
	} else {
		// 如果不允许覆盖且文件存在
		if !overwrite && !os.IsNotExist(err) {
			// 为文件名添加时间戳，创建新版本
			timestamp := time.Now().Format("20060102-150405")
			// 提取文件扩展名
			ext := filepath.Ext(fileName)
			// 提取文件名（不包含扩展名）
			name := strings.TrimSuffix(filepath.Base(fileName), ext)
			// 构建新的文件名，包含时间戳
			fileName = filepath.Join(filepath.Dir(fileName), fmt.Sprintf("%s_%s%s", name, timestamp, ext))
			// 更新文件的完整路径
			fullPath = filepath.Join(*config.LocalDir, fileName)
		}
	}
	// 创建文件
	file, err := os.Create(fullPath)
	// 检查文件创建是否出错
	if err != nil {
		return "", "", fmt.Errorf("创建文件失败: %w", err)
	}
	// 关闭文件，确保资源释放
	defer file.Close()
	// 将请求体中的内容复制到文件中
	if _, err := io.Copy(file, body); err != nil {
		return "", "", fmt.Errorf("保存文件失败: %w", err)
	}
	// 同步文件内容到磁盘
	if err := file.Sync(); err != nil {
		log.Printf("Save请求[ERROR] -> 同步失败: %s, %v", fullPath, err)
	}
	// 记录保存成功日志
	if *config.Developer {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Save请求 -> 成功保存文件: %s, 覆盖: %t", fullPath, overwrite)
		log.Printf("%s", strings.Repeat("-=", 28))
	}
	return fileName, fullPath, nil
}

// DecodeFileName 解码base64编码的文件名
func DecodeFileName(encodedName string) (string, error) {
	// 检查文件名是否为空
	if encodedName == "" {
		return "", fmt.Errorf("缺少文件名")
	}
	// 对编码后的文件名进行解码
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedName)
	// 检查解码是否出错
	if err != nil {
		return "", fmt.Errorf("文件名解码错误: %w", err)
	}
	// 将解码后的字节转换为字符串
	fileName := string(decodedBytes)
	// 再次检查文件名是否为空
	if fileName == "" {
		return "", fmt.Errorf("文件名解码后为空")
	}
	return fileName, nil
}

// FileLocks 用于存储文件路径对应的互斥锁
var FileLocks sync.Map

// GetFileLock 获取指定文件路径的互斥锁
func GetFileLock(filePath string) *sync.Mutex {
	lock, _ := FileLocks.LoadOrStore(filePath, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
