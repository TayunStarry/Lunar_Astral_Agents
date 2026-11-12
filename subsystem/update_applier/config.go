package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// defaultExcludePatterns 更新包默认排除清单（日志、临时文件、版本库与构建产物）。
var defaultExcludePatterns = []string{
	"*.log", "*.tmp", "*.bak", "*.part",
	".git/", "node_modules/", "__pycache__/", "*.pyc", "*.pyo", "*.egg-info/",
	"dist/", "build/",
	".DS_Store", "*.ts", "*.go",
}

// loadReleaseConfig 从程序根的 local_data/config/release.json 读取更新器配置。
func loadReleaseConfig(root string) (*ReleaseConfig, error) {
	path := filepath.Join(root, "local_data", "config", "release.json")
	var cfg ReleaseConfig
	if err := readJSONFile(path, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("未找到更新器配置: %s", path)
		}
		return nil, err
	}
	return &cfg, nil
}

// resolveRepoRoot 解析程序根目录：配置里的 target_dir 为相对路径时以可执行文件目录为基准。
// 空值或 "." 表示可执行文件所在目录。
func resolveRepoRoot(cfg *ReleaseConfig) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := filepath.Dir(exe)
	root := cfg.TargetDir
	if root == "" || root == "." {
		return base, nil
	}
	if filepath.IsAbs(root) {
		return root, nil
	}
	return filepath.Abs(filepath.Join(base, root))
}

// validateRepoConfig 校验仓库配置；needToken 为 true 时要求令牌非空。
func validateRepoConfig(cfg *ReleaseConfig, needToken bool) error {
	if cfg.GiteeOwner == "" || cfg.GiteeRepo == "" {
		return fmt.Errorf("未配置 Gitee 仓库（gitee_owner / gitee_repo 为空）")
	}
	if needToken && cfg.GiteeToken == "" {
		return fmt.Errorf("未提供 Gitee 令牌（发布需要 Release 读写权限）")
	}
	return nil
}

// excludePatterns 返回生效的排除规则。
func excludePatterns(cfg *ReleaseConfig) []string {
	if len(cfg.Exclude) > 0 {
		return cfg.Exclude
	}
	return defaultExcludePatterns
}

// excludeDirMarkers 返回「目录级排除」标记文件名。
func excludeDirMarkers(cfg *ReleaseConfig) []string {
	if len(cfg.ExcludeDirsWith) > 0 {
		return cfg.ExcludeDirsWith
	}
	return []string{"metadata.json"}
}

// packSizeMB 返回分卷大小（MB），默认 95。
func packSizeMB(cfg *ReleaseConfig) int {
	if cfg.PackSizeMB > 0 {
		return cfg.PackSizeMB
	}
	return 95
}

// packLevel 返回压缩等级，默认 5。
func packLevel(cfg *ReleaseConfig) int {
	if cfg.PackLevel >= 1 && cfg.PackLevel <= 9 {
		return cfg.PackLevel
	}
	return 5
}

// backupKeepCount 返回备份保留数量，默认 3。
func backupKeepCount(cfg *ReleaseConfig) int {
	if cfg.BackupKeep > 0 {
		return cfg.BackupKeep
	}
	return 3
}
