package component

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func find7zPath() (string, error) {
	paths := GetSevenZipPaths()
	for _, path := range paths {
		if fileExists(path) {
			return path, nil
		}
	}

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

	return "", fmt.Errorf("未找到7z命令行工具")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
