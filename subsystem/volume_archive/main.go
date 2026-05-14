package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"volume_archive/component"
)

func main() {
	configPath := flag.String("config", "local_data/lunar_config.json", "配置文件路径")
	outputPath := flag.String("output_path", "", "输出文件基础名称")
	partSizeMB := flag.Int("part_size_mb", 0, "分卷大小（MB）")
	compressionLevel := flag.Int("compression_level", 0, "压缩级别（0-9）")
	packagePlan := flag.String("package_plan", "", "打包计划名称")

	flag.Parse()

	params := &component.ExecuteParams{
		ConfigPath:       *configPath,
		OutputPath:       *outputPath,
		PartSizeMB:       *partSizeMB,
		CompressionLevel: *compressionLevel,
		PackagePlan:      *packagePlan,
		StartTime:        time.Now(),
	}

	if err := applyDefaults(params); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	if err := component.Execute(params); err != nil {
		fmt.Fprintf(os.Stderr, "执行失败: %v\n", err)
		os.Exit(1)
	}
}

func applyDefaults(params *component.ExecuteParams) error {
	if err := component.LoadPackageConfig(params.ConfigPath); err != nil {
		return fmt.Errorf("加载配置失败: %v", err)
	}

	defaults := component.GlobalConfig.ProjectArchiving.Defaults

	if params.OutputPath == "" {
		params.OutputPath = defaults.OutputPath
	}
	if params.PartSizeMB == 0 {
		params.PartSizeMB = defaults.PartSizeMB
	}
	if params.CompressionLevel == 0 {
		params.CompressionLevel = defaults.CompressionLevel
	}
	if params.PackagePlan == "" {
		params.PackagePlan = defaults.PackagePlan
	}

	return nil
}
