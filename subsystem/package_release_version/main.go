package main

import (
	"flag"
	"log"
)

func main() {
	// 解析所有命令行参数
	flag.Parse()
	// 尝试加载配置文件，若失败则记录警告并使用内置默认配置
	if err := LoadPackageConfig(*configPath); err != nil {
		log.Printf("警告: 加载配置文件失败: %v", err)
		log.Printf("将使用内置配置")
	}
	// 记录用户显式指定的命令行参数
	specified := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		specified[f.Name] = true
	})
	// 获取内置的默认配置值
	defaultOut, defaultPartSize, defaultCompress, defaultPackage := GetDefaultConfig()
	// 若用户未指定 output_path，则使用默认值
	if !specified["output_path"] {
		*OutputPath = defaultOut
	}
	// 若用户未指定 part_size_mb，则使用默认值
	if !specified["part_size_mb"] {
		*PartSizeMB = defaultPartSize
	}
	// 若用户未指定 compression_level，则使用默认值
	if !specified["compression_level"] {
		*CompressionLevel = defaultCompress
	}
	// 若用户未指定 package_level，则使用默认值
	if !specified["package_level"] {
		*PackageLevel = defaultPackage
	}
	// 若已设置输出路径且分卷大小不为零，则执行打包流程
	if *OutputPath != "" && *PartSizeMB != 0 {
		ExecutePackageProcess()
		return
	}
	// 否则输出错误并退出程序
	log.Fatal("请至少指定<输出基础名>和<分卷大小>")
}
