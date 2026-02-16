package main

import (
	"flag" // 用于解析命令行参数
)

var (
	configPath = flag.String("config", "", "打包配置文件路径")

	SystemDevMode = flag.Bool("system_dev_mode", false, "是否使用调试模式")

	OutputPath = flag.String("output_path", "Lunar-Astral-Agents", "输出文件的基础名称（如需开启打包功能，需指定该参数）")

	PartSizeMB = flag.Int("part_size_mb", 2048, "分卷大小(MB)")

	CompressionLevel = flag.Int("compression_level", 5, "压缩级别 (0-9) \n0表示不压缩，9表示固实压缩")

	PackageLevel = flag.Int("package_level", 3, "打包级别 (1-3)\n"+
		"  1: 核心文件 (可执行文件、网页、配置文件)\n"+
		"  2: 级别1 + 扩展程序\n"+
		"  3: 级别2 + 服务器文件 (所有文件)")
)
