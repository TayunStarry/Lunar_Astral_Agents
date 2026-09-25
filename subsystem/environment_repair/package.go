package main

import (
	"environment_repair/component"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ListArchiveConfigs 列出当前工作目录与 local_data 下的 JSON 配置文件（TUI 选择列表用）
func ListArchiveConfigs() []string {
	var jsonFiles []string
	seen := make(map[string]bool)

	addIfExists := func(path string) {
		if !seen[path] {
			seen[path] = true
			jsonFiles = append(jsonFiles, path)
		}
	}

	entries, err := os.ReadDir(".")
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				addIfExists(entry.Name())
			}
		}
	}

	localEntries, localErr := os.ReadDir("local_data")
	if localErr == nil {
		for _, entry := range localEntries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				addIfExists(filepath.Join("local_data", entry.Name()))
			}
		}
	}
	return jsonFiles
}

// LoadArchiveConfig 加载打包配置文件（TUI 归档屏调用）
func LoadArchiveConfig(configPath string) (*component.ArchiveConfig, error) {
	return component.LoadArchiveConfig(configPath)
}

// RunArchive 执行打包流程（由 TUI 在 goroutine 中调用，日志与进度经钩子推送）
func RunArchive(configPath string, config *component.ArchiveConfig) error {
	params := &component.ExecuteParams{
		ConfigPath: configPath,
		Config:     config,
		StartTime:  time.Now(),
	}
	return component.Execute(params)
}

// configSummaryLines 渲染配置摘要行（TUI 摘要视图用）
func configSummaryLines(config *component.ArchiveConfig, configPath string) []string {
	lines := []string{
		fmt.Sprintf("配置文件: %s", configPath),
		fmt.Sprintf("包含路径: %d 项  ·  排除规则: %d 项", len(config.Import), len(config.Exclude)),
		fmt.Sprintf("输出路径: %s", config.Output),
		fmt.Sprintf("分卷大小: %d MB  ·  压缩等级: %d", config.Size, config.Level),
		fmt.Sprintf("7z 搜索路径: %d 项", len(config.Archive)),
	}
	return lines
}
