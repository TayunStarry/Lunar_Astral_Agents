package component

import (
	"encoding/json"
	"fmt"
	"os"
)

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

	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	if pa, ok := rawMap["project_archiving"].(map[string]interface{}); ok {
		for key, value := range pa {
			if key == "exclude" || key == "sevenzip_paths" || key == "defaults" {
				continue
			}
			if paths, ok := value.([]interface{}); ok {
				var strPaths []string
				for _, p := range paths {
					if str, ok := p.(string); ok {
						strPaths = append(strPaths, str)
					}
				}
				config.ProjectArchiving.Plans[key] = strPaths
			}
		}
	}

	GlobalConfig = config
	return nil
}

func GetExcludePatterns() []string {
	if GlobalConfig == nil {
		return []string{}
	}
	return GlobalConfig.ProjectArchiving.Exclude
}

func GetSevenZipPaths() []string {
	if GlobalConfig == nil {
		return []string{}
	}
	return GlobalConfig.ProjectArchiving.SevenZipPaths
}

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

func matchPattern(pattern, name string) (bool, error) {
	if pattern == "*.log" || pattern == "*.tmp" || pattern == "*.bak" ||
		pattern == "*.pyc" || pattern == "*.pyo" || pattern == "*.ts" || pattern == "*.go" {
		ext := pattern[1:]
		if len(name) >= len(ext) && name[len(name)-len(ext):] == ext {
			return true, nil
		}
		return false, nil
	}

	if pattern == ".DS_Store" {
		return name == ".DS_Store", nil
	}

	if pattern == "node_modules" || pattern == "__pycache__" ||
		pattern == "dist" || pattern == "build" {
		return name == pattern, nil
	}

	if pattern == ".git" {
		return name == ".git", nil
	}

	if pattern == "*.egg-info" {
		ext := ".egg-info"
		if len(name) >= len(ext) && name[len(name)-len(ext):] == ext {
			return true, nil
		}
		return false, nil
	}

	return false, nil
}
