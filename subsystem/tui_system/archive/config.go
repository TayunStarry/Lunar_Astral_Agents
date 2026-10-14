// Package archive 提供 7z 分卷打包归档能力
// （迁移自 environment_repair/component，逻辑保持一致：源扫描/排除匹配/7z 进度解析/旧分卷清理）。
package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config 扁平化的打包配置结构（JSON Schema）
type Config struct {
	Import  []string `json:"import"`  // 需包含在打包范围内的文件/文件夹路径
	Exclude []string `json:"exclude"` // 从打包范围中排除的文件/目录模式
	Archive []string `json:"archive"` // 7z.exe 可执行文件的搜索路径列表
	Output  string   `json:"output"`  // 压缩包输出目录路径及基础文件名
	Size    int      `json:"size"`    // 分卷压缩单包大小（MB）
	Level   int      `json:"level"`   // 压缩等级（1-9）
}

// ExecuteParams 运行时参数
type ExecuteParams struct {
	ConfigPath string  // 配置文件路径
	Config     *Config // 已加载的配置
	StartTime  time.Time
}

// globalConfig 全局配置实例（供排除匹配/7z 路径查询读取）
var globalConfig *Config

// ValidateParams 校验运行时参数合法性
func ValidateParams(params *ExecuteParams) error {
	if params.Config == nil {
		return fmt.Errorf("配置未加载")
	}
	if params.Config.Output == "" {
		return fmt.Errorf("输出路径不能为空")
	}
	if params.Config.Size <= 0 {
		return fmt.Errorf("分卷大小必须大于0")
	}
	if params.Config.Level < 1 || params.Config.Level > 9 {
		return fmt.Errorf("压缩级别必须在1-9范围内")
	}
	if _, err := os.Stat(params.ConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("配置文件不存在: %s", params.ConfigPath)
	}
	return nil
}

// LoadConfig 加载扁平 JSON 配置文件，设置默认值
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 %s: %v", configPath, err)
	}

	config := &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.Size == 0 {
		config.Size = 2048
	}
	if config.Level == 0 {
		config.Level = 5
	}

	globalConfig = config
	return config, nil
}

// GetExcludePatterns 获取排除规则列表
func GetExcludePatterns() []string {
	if globalConfig == nil {
		return []string{}
	}
	return globalConfig.Exclude
}

// GetSevenZipPaths 获取 7z 工具搜索路径列表
func GetSevenZipPaths() []string {
	if globalConfig == nil {
		return []string{}
	}
	return globalConfig.Archive
}

// IsExcluded 检查文件/目录名是否匹配排除规则
func IsExcluded(name string, isDir bool) bool {
	patterns := GetExcludePatterns()
	for _, pattern := range patterns {
		isDirPattern := len(pattern) > 0 && pattern[len(pattern)-1] == '/'
		patternWithoutSlash := pattern
		if isDirPattern {
			patternWithoutSlash = pattern[:len(pattern)-1]
		}

		if isDir && isDirPattern {
			if matched, _ := matchPattern(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDir && !isDirPattern {
			if matched, _ := matchPattern(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDirPattern {
			if matched, _ := matchPattern(patternWithoutSlash, name); matched {
				return true
			}
		}
	}
	return false
}

// matchPattern 自定义通配符匹配（支持 *.ext、目录名、精确文件名）
func matchPattern(pattern, name string) (bool, error) {
	// 通配符扩展名匹配：*.log, *.tmp, *.bak, *.pyc, *.pyo, *.ts, *.go
	if len(pattern) > 1 && pattern[0] == '*' && pattern[1] == '.' {
		ext := pattern[1:]
		if len(name) >= len(ext) && name[len(name)-len(ext):] == ext {
			return true, nil
		}
		return false, nil
	}

	// 精确文件名匹配
	if pattern == ".DS_Store" {
		return name == ".DS_Store", nil
	}

	// 目录名匹配
	knownDirs := []string{"node_modules", "__pycache__", "dist", "build", ".git"}
	for _, d := range knownDirs {
		if pattern == d {
			return name == d, nil
		}
	}

	// .egg-info 后缀匹配
	if pattern == "*.egg-info" {
		ext := ".egg-info"
		if len(name) >= len(ext) && name[len(name)-len(ext):] == ext {
			return true, nil
		}
		return false, nil
	}

	return false, nil
}
