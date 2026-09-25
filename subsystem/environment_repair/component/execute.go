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

	uiLogf("========================================")
	uiLogf("  项目打包工具启动")
	uiLogf("  启动时间: %s", params.StartTime.Format("2006-01-02 15:04:05"))
	uiLogf("========================================")

	sources, err := GetSources(params.Config.Import)
	if err != nil {
		return fmt.Errorf("获取源文件失败: %v", err)
	}

	uiLogf("  输出路径: %s", params.Config.Output)
	uiLogf("  分卷大小: %d MB", params.Config.Size)
	uiLogf("  压缩级别: %d", params.Config.Level)

	cleanOldParts(params.Config.Output)

	if err := createVolume(sources, params.Config.Output, params.Config.Size, params.Config.Level); err != nil {
		return fmt.Errorf("创建分卷失败: %v", err)
	}

	elapsed := time.Since(params.StartTime)
	uiLogf("========================================")
	uiLogf("  打包完成！")
	uiLogf("  总耗时: %s", elapsed)
	uiLogf("========================================")

	return nil
}
