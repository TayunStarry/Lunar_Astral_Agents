package component

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolvePath 将相对路径解析为绝对路径
func ResolvePath(relativePath string) (string, error) {
	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

// GetBaseDir 获取源文件列表的基准目录（第一个源文件所在的目录）
func GetBaseDir(sources []string) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("源文件列表为空")
	}

	firstSource := sources[0]
	absPath, err := filepath.Abs(firstSource)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return absPath, nil
	}

	return filepath.Dir(absPath), nil
}