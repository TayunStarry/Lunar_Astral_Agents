package component

import "fmt"

// getSourcesByPlan 根据传入的打包计划名称获取对应的源文件列表
func getSourcesByPlan(plan string) ([]string, error) {
	// 尝试从配置文件获取
	sources, err := GetSourcesByPlan(plan)
	if err != nil {
		// 没有配置时终止程序执行
		return nil, fmt.Errorf("配置加载失败，程序终止: %v", err)
	}
	return sources, nil
}
