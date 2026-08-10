package component

import (
	"fmt"
	"os"
	"time"
)

// ArchiveConfig 扁平化的打包配置结构（JSON Schema）
type ArchiveConfig struct {
	Import  []string `json:"import"`  // 需包含在打包范围内的文件/文件夹路径
	Exclude []string `json:"exclude"` // 从打包范围中排除的文件/目录模式
	Archive []string `json:"archive"` // 7z.exe 可执行文件的搜索路径列表
	Output  string   `json:"output"`  // 压缩包输出目录路径及基础文件名
	Size    int      `json:"size"`    // 分卷压缩单包大小（MB）
	Level   int      `json:"level"`   // 压缩等级（1-9）
}

// ExecuteParams 运行时参数
type ExecuteParams struct {
	ConfigPath string         // 配置文件路径
	Config     *ArchiveConfig // 已加载的配置
	StartTime  time.Time      // 启动时间
}

// GlobalConfig 全局配置实例
var GlobalConfig *ArchiveConfig

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