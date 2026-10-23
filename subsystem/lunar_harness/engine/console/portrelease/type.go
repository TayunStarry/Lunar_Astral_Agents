// Package portrelease 提供端口占用扫描与进程终止能力
// （迁移自 environment_repair/release.go，逻辑保持一致：PowerShell 优先 + netstat 回退）。
package portrelease

// ProcessInfo 存储进程信息
type ProcessInfo struct {
	PID     int    // 进程ID
	Port    int    // 占用端口
	Name    string // 进程名称
	CmdLine string // 启动命令行
}

// portRange 端口扫描范围
type portRange struct {
	Start int
	End   int
}
