package component

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// find7zPath 按配置路径 → PATH 环境变量的顺序查找 7z 可执行文件
func find7zPath() (string, error) {
	// 1. 按配置文件中指定的路径搜索
	paths := GetSevenZipPaths()
	for _, path := range paths {
		if fileExists(path) {
			return path, nil
		}
	}

	// 2. 在系统 PATH 中搜索
	cmd := exec.Command("where", "7z")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			path := strings.TrimSpace(line)
			if path != "" {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("未找到7z命令行工具，请安装 7-Zip 或在配置文件的 archive 字段中指定路径")
}

// fileExists 检查文件或目录是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}