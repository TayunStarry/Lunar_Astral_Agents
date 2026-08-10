package component

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadArchiveConfig 加载扁平 JSON 配置文件，设置默认值
func LoadArchiveConfig(configPath string) (*ArchiveConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 %s: %v", configPath, err)
	}

	config := &ArchiveConfig{}
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

	GlobalConfig = config
	return config, nil
}

// GetExcludePatterns 获取排除规则列表
func GetExcludePatterns() []string {
	if GlobalConfig == nil {
		return []string{}
	}
	return GlobalConfig.Exclude
}

// GetSevenZipPaths 获取 7z 工具搜索路径列表
func GetSevenZipPaths() []string {
	if GlobalConfig == nil {
		return []string{}
	}
	return GlobalConfig.Archive
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
			if matched, _ := filepathMatch(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDir && !isDirPattern {
			if matched, _ := filepathMatch(patternWithoutSlash, name); matched {
				return true
			}
		} else if !isDirPattern {
			if matched, _ := filepathMatch(patternWithoutSlash, name); matched {
				return true
			}
		}
	}
	return false
}

func filepathMatch(pattern, name string) (bool, error) {
	return matchPattern(pattern, name)
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