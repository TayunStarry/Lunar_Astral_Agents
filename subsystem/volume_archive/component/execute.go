package component

import (
	"fmt"
	"time"
)

func Execute(params *ExecuteParams) error {
	params.StartTime = time.Now()

	if err := ValidateParams(params); err != nil {
		return fmt.Errorf("参数验证失败: %v", err)
	}

	PrintInfo("========================================")
	PrintInfo("项目打包工具启动")
	PrintInfo("启动时间: %s", params.StartTime.Format("2006-01-02 15:04:05"))
	PrintInfo("========================================")

	if err := LoadPackageConfig(params.ConfigPath); err != nil {
		return fmt.Errorf("加载配置失败: %v", err)
	}

	PrintInfo("配置文件加载成功: %s", params.ConfigPath)

	sources, err := GetSourcesByPlan(params.PackagePlan)
	if err != nil {
		return fmt.Errorf("获取源文件失败: %v", err)
	}

	PrintInfo("打包计划: %s", params.PackagePlan)
	PrintInfo("输出路径: %s", params.OutputPath)
	PrintInfo("分卷大小: %d MB", params.PartSizeMB)
	PrintInfo("压缩级别: %d", params.CompressionLevel)

	cleanOldParts(params.OutputPath)

	if err := createVolume(sources, params.OutputPath, params.PartSizeMB, params.CompressionLevel); err != nil {
		return fmt.Errorf("创建分卷失败: %v", err)
	}

	elapsed := time.Since(params.StartTime)
	PrintInfo("========================================")
	PrintSuccess("打包完成！")
	PrintInfo("总耗时: %s", elapsed)
	PrintInfo("========================================")

	return nil
}
