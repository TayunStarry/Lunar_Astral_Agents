package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PackageLevelConfig 打包级别的配置
type PackageLevelConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}

// PackageConfig 打包配置文件结构
type PackageConfig struct {
	ProjectArchiving struct {
		PackageLevels map[string]PackageLevelConfig `json:"package_levels"`
		SevenZipPaths []string                      `json:"sevenzip_paths"`
		Defaults      struct {
			OutputPath       string `json:"output_path"`
			PartSizeMB       int    `json:"part_size_mb"`
			CompressionLevel int    `json:"compression_level"`
			PackageLevel     int    `json:"package_level"`
		} `json:"defaults"`
	} `json:"project_archiving"`
}

var packageConfig *PackageConfig

// LoadPackageConfig 加载打包配置文件
func LoadPackageConfig(configPath string) error {
	// 如果未指定配置文件路径，使用默认路径
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("获取可执行文件路径失败: %v", err)
		}
		exeDir := filepath.Dir(exePath)
		configPath = filepath.Join(exeDir, "local_data/lunar_config.json")
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败 %s: %v", configPath, err)
	}

	// 解析JSON
	config := &PackageConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 验证配置
	if len(config.ProjectArchiving.PackageLevels) == 0 {
		return fmt.Errorf("配置文件中未定义打包级别")
	}

	if len(config.ProjectArchiving.SevenZipPaths) == 0 {
		// 如果没有配置，使用默认路径
		config.ProjectArchiving.SevenZipPaths = []string{
			"./7z/7z.exe",
			"C:/Program Files/7-Zip/7z.exe",
			"C:/Program Files (x86)/7-Zip/7z.exe",
		}
	}

	packageConfig = config
	return nil
}

// GetPackageConfig 获取打包配置
func GetPackageConfig() (*PackageConfig, error) {
	if packageConfig == nil {
		return nil, fmt.Errorf("打包配置未加载")
	}
	return packageConfig, nil
}

// GetSourcesByLevel 根据级别获取源文件列表（使用配置文件）
func GetSourcesByLevel(level int) ([]string, error) {
	config, err := GetPackageConfig()
	if err != nil {
		return nil, err
	}

	// 将级别转换为字符串
	levelKey := fmt.Sprintf("%d", level)
	levelConfig, exists := config.ProjectArchiving.PackageLevels[levelKey]
	if !exists {
		return nil, fmt.Errorf("无效的打包级别: %d", level)
	}

	fmt.Printf("打包级别 %d (%s): %s\n", level, levelConfig.Name, levelConfig.Description)
	fmt.Println("包含以下文件:")
	for i, src := range levelConfig.Sources {
		fmt.Printf("  %d. %s\n", i+1, src)
	}
	fmt.Println()

	return levelConfig.Sources, nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() (string, int, int, int) {
	config, err := GetPackageConfig()
	if err != nil {
		// 如果配置加载失败，返回硬编码的默认值
		return "Lunar-Astral-Agents", 2048, 5, 3
	}
	return config.ProjectArchiving.Defaults.OutputPath,
		config.ProjectArchiving.Defaults.PartSizeMB,
		config.ProjectArchiving.Defaults.CompressionLevel,
		config.ProjectArchiving.Defaults.PackageLevel
}

// GetSevenZipPaths 获取7z路径列表
func GetSevenZipPaths() []string {
	config, err := GetPackageConfig()
	if err != nil {
		// 返回硬编码的默认路径
		return []string{
			"./archive/7z.exe",
			"C:/Program Files/7-Zip/7z.exe",
			"C:/Program Files (x86)/7-Zip/7z.exe",
		}
	}
	return config.ProjectArchiving.SevenZipPaths
}
