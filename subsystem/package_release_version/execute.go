package main

import (
	"flag" // 用于解析命令行参数
	"fmt"  // 用于格式化输出
	"os"   // 用于操作系统相关操
	"time" // 用于时间操作
)

// ExecutePackageProcess 执行打包器的主函数，负责整个打包流程的控制和错误处理
// 如果参数不合法或运行时发生错误，会显示错误提示并退出程序
// 如果创建分卷压缩文件成功，会输出分卷压缩文件的路径和打包耗时，然后退出程序
func ExecutePackageProcess() {
	// 检查必要的配置参数是否合法，输出基准路径不能为空且分卷大小必须大于0
	if *OutputPath == "" || *PartSizeMB <= 0 {
		fmt.Println("错误: 必须指定输出文件名称和分卷大小")
		// 显示命令行参数的使用说明
		flag.Usage()
		// 参数不合法，程序异常退出
		os.Exit(1)
	}
	// 检查压缩级别是否在合法范围内（0-9）
	if *CompressionLevel < 0 || *CompressionLevel > 9 {
		fmt.Println("错误: 压缩级别必须在 0-9 范围内")
		// 压缩级别不合法，程序异常退出
		os.Exit(1)
	}
	// 记录脚本开始执行的时间
	startTime := time.Now()
	// 格式化开始时间为易读的字符串
	startStr := startTime.Format("2006-01-02 15:04:05")
	fmt.Printf("脚本开始时间: %s\n\n", startStr)
	// 获取7z命令行工具的路径
	sevenZipPath, err := find7zPath()
	if err != nil {
		fmt.Println("错误: 未找到7z命令行工具")
		fmt.Println("请从 https://www.7-zip.org/ 下载安装7-Zip")
		fmt.Println("如果已安装，请将其安装路径（如 C:\\Program Files\\7-Zip）添加到系统PATH环境变量")
		// 未找到7z工具，程序异常退出
		os.Exit(1)
	}
	// 输出打包配置信息
	fmt.Printf("输出基准: %s\n", *OutputPath)
	fmt.Printf("分卷大小: %d MB\n", *PartSizeMB)
	fmt.Printf("压缩级别: %d\n", *CompressionLevel)
	fmt.Printf("打包级别: %d\n", *PackageLevel)
	// 根据打包级别获取对应的源文件列表
	sources, err := getSourcesByLevel(*PackageLevel)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		// 获取源文件列表失败，程序异常退出
		os.Exit(1)
	}
	// 清理旧的分卷文件
	cleanOldParts(*OutputPath)
	// 使用7z创建分卷压缩文件
	if err := createVolume(sevenZipPath, sources, *OutputPath, *PartSizeMB, *CompressionLevel); err != nil {
		fmt.Printf("压缩失败: %v\n", err)
		// 压缩过程失败，程序异常退出
		os.Exit(1)
	}
	// 记录脚本结束执行的时间
	endTime := time.Now()
	// 格式化结束时间为易读的字符串
	endStr := endTime.Format("2006-01-02 15:04:05")
	// 计算脚本执行的总耗时
	totalDuration := endTime.Sub(startTime)
	fmt.Printf("脚本结束时间: %s\n", endStr)
	fmt.Printf("总耗时: %s\n", formatDuration(totalDuration))
	// 脚本正常执行结束，程序正常退出
	os.Exit(0)
}
