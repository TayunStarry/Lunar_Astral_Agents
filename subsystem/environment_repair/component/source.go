package component

import (
	"fmt"
	"path/filepath"
)

// GetSources 将配置中的 import 路径列表解析为绝对路径，过滤不存在的路径
func GetSources(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("import 列表为空，请在配置文件中指定需打包的文件或目录")
	}

	var sources []string
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("  [WARN] 无法解析路径 %s: %v\n", path, err)
			continue
		}

		if !fileExists(absPath) {
			fmt.Printf("  [WARN] 路径不存在: %s\n", absPath)
			continue
		}

		sources = append(sources, absPath)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("没有有效的源文件可打包")
	}

	return sources, nil
}