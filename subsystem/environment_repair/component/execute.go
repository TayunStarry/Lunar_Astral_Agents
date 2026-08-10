package component

import (
	"fmt"
	"time"
)

// Execute 执行打包流程：验证参数 → 获取源文件 → 清理旧文件 → 压缩
func Execute(params *ExecuteParams) error {
	params.StartTime = time.Now()

	if err := ValidateParams(params); err != nil {
		return fmt.Errorf("参数验证失败: %v", err)
	}

	fmt.Println("========================================")
	fmt.Println("  项目打包工具启动")
	fmt.Printf("  启动时间: %s\n", params.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Println("========================================")

	sources, err := GetSources(params.Config.Import)
	if err != nil {
		return fmt.Errorf("获取源文件失败: %v", err)
	}

	fmt.Printf("  输出路径: %s\n", params.Config.Output)
	fmt.Printf("  分卷大小: %d MB\n", params.Config.Size)
	fmt.Printf("  压缩级别: %d\n", params.Config.Level)

	cleanOldParts(params.Config.Output)

	if err := createVolume(sources, params.Config.Output, params.Config.Size, params.Config.Level); err != nil {
		return fmt.Errorf("创建分卷失败: %v", err)
	}

	elapsed := time.Since(params.StartTime)
	fmt.Println("========================================")
	fmt.Println("  打包完成！")
	fmt.Printf("  总耗时: %s\n", elapsed)
	fmt.Println("========================================")

	return nil
}