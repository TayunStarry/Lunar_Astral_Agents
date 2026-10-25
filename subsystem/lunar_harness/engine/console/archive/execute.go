package archive

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/sound"
)

// ListConfigs 返回候选打包配置：默认配置（local_data/archive_config.json，存在时）置于最前，
// 其后为工作目录 data/ 下的额外 *.json 配置（便于用户自定义多套打包方案）。
func ListConfigs() []string {
	configs := make([]string, 0, 4)
	if fileExists(DefaultConfigPath) {
		configs = append(configs, DefaultConfigPath)
	}

	entries, err := os.ReadDir("data")
	if err != nil {
		return configs
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			configs = append(configs, filepath.Join("data", entry.Name()))
		}
	}
	return configs
}

// Execute 执行打包流程：验证参数 → 获取源文件 → 清理旧文件 → 压缩。
// configPath 为配置文件路径（用于展示与校验）；cfg 为已加载配置（允许调用方程序化构造，如更新包发布）。
func Execute(configPath string, cfg *Config) error {
	params := &ExecuteParams{
		ConfigPath: configPath,
		Config:     cfg,
		StartTime:  nowTime(),
	}

	if err := ValidateParams(params); err != nil {
		return fmt.Errorf("参数验证失败: %v", err)
	}

	// 程序化构造时同步全局配置（排除匹配/7z 路径查询依赖）
	globalConfig = params.Config

	logf("========================================")
	logf("  项目打包工具启动")
	logf("  启动时间: %s", params.StartTime.Format("2006-01-02 15:04:05"))
	logf("========================================")

	sources, err := GetSources(params.Config.Import)
	if err != nil {
		return fmt.Errorf("获取源文件失败: %v", err)
	}

	logf("  输出路径: %s", params.Config.Output)
	logf("  分卷大小: %d MB", params.Config.Size)
	logf("  压缩级别: %d", params.Config.Level)

	cleanOldParts(params.Config.Output)

	if err := createVolume(sources, params.Config.Output, params.Config.Size, params.Config.Level); err != nil {
		return fmt.Errorf("创建分卷失败: %v", err)
	}

	elapsed := sinceTime(params.StartTime)
	logf("========================================")
	logf("  打包完成！")
	logf("  总耗时: %s", elapsed)
	logf("========================================")
	// 打包归档属于资源产出：由 Go 层统一播放完成提示音
	sound.Notify()

	return nil
}

// GetSources 将配置中的 import 路径列表解析为绝对路径，过滤不存在的路径
func GetSources(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("import 列表为空，请在配置文件中指定需打包的文件或目录")
	}

	var sources []string
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			logf("  [WARN] 无法解析路径 %s: %v", path, err)
			continue
		}

		if !fileExists(absPath) {
			logf("  [WARN] 路径不存在: %s", absPath)
			continue
		}

		sources = append(sources, absPath)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("没有有效的源文件可打包")
	}

	return sources, nil
}

// find7zPath 按配置路径 → PATH 环境变量的顺序查找 7z 可执行文件。
// 返回绝对路径：createVolume 以 cmd.Dir=源基准目录 启动 7z 进程，
// 相对路径会在子进程的 cwd 下失效。
func find7zPath() (string, error) {
	// 1. 按配置文件中指定的路径搜索
	paths := GetSevenZipPaths()
	for _, path := range paths {
		if fileExists(path) {
			if abs, err := filepath.Abs(path); err == nil {
				return abs, nil
			}
			return path, nil
		}
	}

	// 2. 在系统 PATH 中搜索
	cmd := exec.Command("where", "7z")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			path := strings.TrimSpace(line)
			if path != "" {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("未找到7z命令行工具，请安装 7-Zip 或在配置文件的 archive 字段中指定路径")
}

// fileExists 检查文件或目录是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
