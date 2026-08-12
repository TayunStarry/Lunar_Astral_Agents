package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"environment_repair/component"
)

// runPackageArchive 分卷打包归档 CLI 入口
// 交互流程：列出配置文件 → 用户指定路径 → 加载 → 显示摘要 → 确认 → 执行
func runPackageArchive() {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("  [4] 分卷打包归档")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()

	// 1. 列出当前目录下可用的 JSON 配置文件
	listConfigFiles()

	// 2. 提示用户输入配置文件路径
	configPath := promptConfigPath()
	if configPath == "" {
		return
	}

	// 3. 加载配置文件
	fmt.Printf("\n正在加载配置文件: %s\n", configPath)
	config, err := component.LoadArchiveConfig(configPath)
	if err != nil {
		fmt.Printf("\n✗ 加载配置失败: %v\n", err)
		return
	}

	// 4. 显示配置摘要
	printConfigSummary(config, configPath)

	// 5. 确认执行
	if !confirmExecution() {
		fmt.Println("\n已取消打包操作。")
		return
	}

	// 6. 执行打包
	fmt.Println()
	params := &component.ExecuteParams{
		ConfigPath: configPath,
		Config:     config,
		StartTime:  time.Now(),
	}

	if err := component.Execute(params); err != nil {
		fmt.Printf("\n✗ 打包失败: %v\n", err)
	} else {
		fmt.Println("\n打包流程完成。")
	}
}

// listConfigFiles 列出当前工作目录下可用的 JSON 配置文件
func listConfigFiles() {
	cwd, _ := os.Getwd()
	fmt.Printf("当前工作目录: %s\n", cwd)
	fmt.Println()

	entries, err := os.ReadDir(".")
	if err != nil {
		return
	}

	var jsonFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			jsonFiles = append(jsonFiles, entry.Name())
		}
	}

	// 同时检查 local_data 目录
	localEntries, localErr := os.ReadDir("local_data")
	if localErr == nil {
		for _, entry := range localEntries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				jsonFiles = append(jsonFiles, filepath.Join("local_data", entry.Name()))
			}
		}
	}

	if len(jsonFiles) > 0 {
		fmt.Println("  发现以下 JSON 配置文件：")
		for _, f := range jsonFiles {
			fmt.Printf("    · %s\n", f)
		}
		fmt.Println()
	}
}

// promptConfigPath 提示用户输入配置文件路径
func promptConfigPath() string {
	scanner := bufio.NewScanner(os.Stdin)

	defaultPath := "local_data/archive_config.json"
	fmt.Printf("请输入配置文件路径 [默认 %s]: ", defaultPath)

	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			// 去除可能的引号（拖拽文件时可能带引号）
			input = strings.Trim(input, "\"'")
			defaultPath = input
		}
	}

	// 检查文件是否存在
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		fmt.Printf("  [WARN] 配置文件不存在: %s\n", defaultPath)
		fmt.Print("  是否继续？(y/N): ")
		if scanner.Scan() {
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				return ""
			}
		} else {
			return ""
		}
	}

	_ = scanner.Err()
	return defaultPath
}

// printConfigSummary 显示配置摘要
func printConfigSummary(config *component.ArchiveConfig, configPath string) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("  配置摘要")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Printf("  配置文件: %s\n", configPath)
	fmt.Printf("  包含路径: %d 项\n", len(config.Import))
	for _, p := range config.Import {
		fmt.Printf("    · %s\n", p)
	}
	fmt.Printf("  排除规则: %d 项\n", len(config.Exclude))
	if len(config.Exclude) > 0 {
		for _, e := range config.Exclude {
			fmt.Printf("    · %s\n", e)
		}
	}
	fmt.Printf("  7z 搜索路径: %d 项\n", len(config.Archive))
	if len(config.Archive) > 0 {
		for _, a := range config.Archive {
			fmt.Printf("    · %s\n", a)
		}
	}
	fmt.Printf("  输出路径: %s\n", config.Output)
	fmt.Printf("  分卷大小: %d MB\n", config.Size)
	fmt.Printf("  压缩等级: %d\n", config.Level)
	fmt.Println(strings.Repeat("─", 48))
}

// confirmExecution 询问用户确认执行
func confirmExecution() bool {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("\n确认开始打包？(Y/n): ")
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "n" || answer == "no" {
			return false
		}
	}
	_ = scanner.Err()
	return true
}