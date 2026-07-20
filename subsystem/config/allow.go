package config

import "flag"

var (
	// AllowDiffusion 是否允许运行扩散生成机制
	AllowDiffusion = flag.Bool("allow-diffusion", true, "是否允许运行扩散生成机制, 默认允许")
	// AllowBrowser 是否允许使用浏览器
	AllowBrowser = flag.Bool("allow-browser", true, "是否允许使用浏览器, 默认允许")
	// ClearPort 启动时自动释放被占用的端口
	ClearPort = flag.Bool("clear-port", false, "启动时自动释放被占用的端口, 默认不释放")
)
