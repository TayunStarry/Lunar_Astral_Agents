package component

import (
	"flag" // 用于解析命令行参数
)

var (
	ConfigPath = flag.String("config", "local_data/lunar_config.json", "打包配置文件路径")

	SystemDevMode = flag.Bool("system_dev_mode", false, "是否使用调试模式")

	OutputPath = flag.String("output_path", "Lunar-Astral-Agents", "输出文件的基础名称（如需开启打包功能，需指定该参数）")

	PartSizeMB = flag.Int("part_size_mb", 2048, "分卷大小(MB)")

	CompressionLevel = flag.Int("compression_level", 5, "压缩级别 (0-9) \n0表示不压缩，9表示固实压缩")

	PackagePlan = flag.String("package_plan", "plan-3", "打包计划")
)
