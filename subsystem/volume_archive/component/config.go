package component

import (
	"fmt"
	"os"
	"time"
)

type ArchivingConfig struct {
	Plans         map[string][]string `json:"-"`
	Exclude       []string            `json:"exclude"`
	SevenZipPaths []string            `json:"sevenzip_paths"`
	Defaults      DefaultConfig       `json:"defaults"`
}

type DefaultConfig struct {
	OutputPath       string `json:"output_path"`
	PartSizeMB       int    `json:"part_size_mb"`
	CompressionLevel int    `json:"compression_level"`
	PackagePlan      string `json:"package_plan"`
}

type PackageConfig struct {
	ProjectArchiving ArchivingConfig `json:"project_archiving"`
}

type ExecuteParams struct {
	ConfigPath       string
	OutputPath       string
	PartSizeMB       int
	CompressionLevel int
	PackagePlan      string
	SystemDevMode    bool
	StartTime        time.Time
}

var (
	GlobalConfig *PackageConfig
)

func ValidateParams(params *ExecuteParams) error {
	if params.OutputPath == "" {
		return fmt.Errorf("输出路径不能为空")
	}
	if params.PartSizeMB <= 0 {
		return fmt.Errorf("分卷大小必须大于0")
	}
	if params.CompressionLevel < 0 || params.CompressionLevel > 9 {
		return fmt.Errorf("压缩级别必须在0-9范围内")
	}
	if _, err := os.Stat(params.ConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("配置文件不存在: %s", params.ConfigPath)
	}
	return nil
}
