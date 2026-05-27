package component

import (
	"fmt"
	"logger"
	"os"
	"path/filepath"
)

func ResolvePath(relativePath string) (string, error) {
	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func GetBaseDir(sources []string) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("源文件列表为空")
	}

	firstSource := sources[0]
	absPath, err := filepath.Abs(firstSource)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return absPath, nil
	}

	return filepath.Dir(absPath), nil
}

func PrintInfo(format string, args ...interface{}) {
	logger.Info("VolumeArchive", format, args...)
}

func PrintWarning(format string, args ...interface{}) {
	logger.Error("VolumeArchive", format, args...)
}

func PrintError(format string, args ...interface{}) {
	logger.Error("VolumeArchive", format, args...)
}

func PrintSuccess(format string, args ...interface{}) {
	logger.Info("VolumeArchive", format, args...)
}
