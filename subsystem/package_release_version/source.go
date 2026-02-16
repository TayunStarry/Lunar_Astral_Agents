package main

import "fmt"

// getSourcesByLevel 根据传入的打包级别获取对应的源文件列表
func getSourcesByLevel(level int) ([]string, error) {
	// 尝试从配置文件获取
	sources, err := GetSourcesByLevel(level)
	if err != nil {
		// 没有配置时终止程序执行
		return nil, fmt.Errorf("配置加载失败，程序终止: %v", err)
	}
	return sources, nil
}