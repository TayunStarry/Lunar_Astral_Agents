package viewer

// store_paths.go 哔哩观影的落盘路径。

import (
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

func localRoot() string {
	dir := strings.TrimSpace(config.LocalDir)
	if dir == "" {
		return "local_data"
	}
	return dir
}

// stateDir 返回哔哩观影的落盘目录（已看/去重共用）。
func stateDir() string {
	return filepath.Join(localRoot(), "database", "bilibili")
}
