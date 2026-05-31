package component

import (
	"fmt"
	"path/filepath"
)

func GetSourcesByPlan(planName string) ([]string, error) {
	if GlobalConfig == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	paths, ok := GlobalConfig.ProjectArchiving.Plans[planName]
	if !ok {
		return nil, fmt.Errorf("未找到打包计划: %s", planName)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("打包计划 %s 为空", planName)
	}

	var sources []string
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			PrintWarning("无法解析路径 %s: %v", path, err)
			continue
		}

		if !fileExists(absPath) {
			PrintWarning("路径不存在: %s (%s)", path, absPath)
			continue
		}

		sources = append(sources, absPath)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("没有有效的源文件")
	}

	return sources, nil
}
