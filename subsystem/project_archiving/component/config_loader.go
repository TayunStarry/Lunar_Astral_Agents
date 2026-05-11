package component

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PackageConfig 打包配置文件结构
type PackageConfig struct {
	ProjectArchiving struct {
		Plans         map[string][]string `json:"-"`
		Plan1         []string            `json:"plan-1"`
		Plan2         []string            `json:"plan-2"`
		Plan3         []string            `json:"plan-3"`
		Exclude       []string            `json:"exclude"`
		SevenZipPaths []string            `json:"sevenzip_paths"`
		Defaults      struct {
			OutputPath       string `json:"output_path"`
			PartSizeMB       int    `json:"part_size_mb"`
			CompressionLevel int    `json:"compression_level"`
			PackagePlan      string `json:"package_plan"`
		} `json:"defaults"`
	} `json:"project_archiving"`
}

var packageConfig *PackageConfig

// LoadPackageConfig 加载打包配置文件
func LoadPackageConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败 %s: %v", configPath, err)
	}

	config := &PackageConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	config.ProjectArchiving.Plans = make(map[string][]string)
	config.ProjectArchiving.Plans["plan-1"] = config.ProjectArchiving.Plan1
	config.ProjectArchiving.Plans["plan-2"] = config.ProjectArchiving.Plan2
	config.ProjectArchiving.Plans["plan-3"] = config.ProjectArchiving.Plan3

	if len(config.ProjectArchiving.Plans) == 0 {
		return fmt.Errorf("配置文件中未定义打包计划")
	}

	if len(config.ProjectArchiving.SevenZipPaths) == 0 {
		config.ProjectArchiving.SevenZipPaths = []string{
			"./7z/7z.exe",
			"C:/Program Files/7-Zip/7z.exe",
			"C:/Program Files (x86)/7-Zip/7z.exe",
		}
	}

	if len(config.ProjectArchiving.Exclude) == 0 {
		config.ProjectArchiving.Exclude = []string{
			"*.log",
			"*.tmp",
			"*.bak",
			".git/",
			"node_modules/",
			"__pycache__/",
			"*.pyc",
			"*.pyo",
			"*.egg-info/",
			"dist/",
			"build/",
			".DS_Store",
			"*.ts",
			"*.go",
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

// GetSourcesByPlan 根据计划名称获取源文件列表（使用配置文件）
func GetSourcesByPlan(plan string) ([]string, error) {
	config, err := GetPackageConfig()
	if err != nil {
		return nil, err
	}

	sources, exists := config.ProjectArchiving.Plans[plan]
	if !exists {
		return nil, fmt.Errorf("无效的打包计划: %s", plan)
	}

	fmt.Printf("打包计划: %s\n", plan)
	fmt.Println("包含以下文件:")
	for i, src := range sources {
		fmt.Printf("  %d. %s\n", i+1, src)
	}

	if len(config.ProjectArchiving.Exclude) > 0 {
		fmt.Println("\n排除规则:")
		for i, pattern := range config.ProjectArchiving.Exclude {
			fmt.Printf("  %d. %s\n", i+1, pattern)
		}
	}
	fmt.Println()

	return sources, nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() (string, int, int, string) {
	config, err := GetPackageConfig()
	if err != nil {
		return "Lunar-Astral-Agents", 2048, 5, "plan-3"
	}
	return config.ProjectArchiving.Defaults.OutputPath,
		config.ProjectArchiving.Defaults.PartSizeMB,
		config.ProjectArchiving.Defaults.CompressionLevel,
		config.ProjectArchiving.Defaults.PackagePlan
}

// GetSevenZipPaths 获取7z路径列表
func GetSevenZipPaths() []string {
	config, err := GetPackageConfig()
	if err != nil {
		return []string{
			"./archive/7z.exe",
			"C:/Program Files/7-Zip/7z.exe",
			"C:/Program Files (x86)/7-Zip/7z.exe",
		}
	}
	return config.ProjectArchiving.SevenZipPaths
}

// GetExcludePatterns 获取排除模式列表
func GetExcludePatterns() []string {
	config, err := GetPackageConfig()
	if err != nil {
		return []string{
			"*.log",
			"*.tmp",
			"*.bak",
			".git/",
			"node_modules/",
			"__pycache__/",
			"*.pyc",
			"*.pyo",
			"*.egg-info/",
			"dist/",
			"build/",
			".DS_Store",
			"*.ts",
			"*.go",
		}
	}
	return config.ProjectArchiving.Exclude
}

// IsExcluded 检查文件或目录是否应该被排除
func IsExcluded(name string, isDir bool) bool {
	patterns := GetExcludePatterns()
	for _, pattern := range patterns {
		isDirPattern := strings.HasSuffix(pattern, "/")
		patternWithoutSlash := strings.TrimSuffix(pattern, "/")

		if isDir && isDirPattern {
			if matched, _ := filepath.Match(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDir && !isDirPattern {
			if matched, _ := filepath.Match(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDirPattern {
			if matched, _ := filepath.Match(patternWithoutSlash, name); matched {
				return true
			}
		}
	}
	return false
}
