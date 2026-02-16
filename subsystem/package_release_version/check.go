package main

import (
	"fmt"     // 用于格式化输入输出
	"os"      // 用于操作文件系统
	"os/exec" // 用于执行外部命令
	"strings" // 用于字符串操作
)

// find7zPath 函数用于查找系统中7z命令行工具的路径。
// 首先会检查默认安装路径，若未找到则通过系统的where命令在PATH环境变量中查找。
// 返回找到的7z工具路径和nil错误，若未找到则返回空字符串和错误信息。
func find7zPath() (string, error) {
	// 首先尝试从配置文件获取路径列表
	paths := GetSevenZipPaths()

	// 遍历所有路径
	for _, path := range paths {
		if fileExists(path) {
			fmt.Printf("找到7z: %s\n", path)
			return path, nil
		}
	}

	// 如果配置文件中未找到，尝试系统的where命令
	cmd := exec.Command("where", "7z")
	output, err := cmd.Output()
	if err == nil {
		path := strings.TrimSpace(strings.Split(string(output), "\n")[0])
		if path != "" {
			fmt.Printf("通过PATH找到7z: %s\n", path)
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到7z命令行工具")
}

// fileExists 函数用于检查指定路径的文件或目录是否存在。
// 返回一个布尔值，表示文件或目录是否存在。
func fileExists(path string) bool {
	// 使用os.Stat获取文件信息，若成功则表示文件存在
	_, err := os.Stat(path)
	return err == nil
}
