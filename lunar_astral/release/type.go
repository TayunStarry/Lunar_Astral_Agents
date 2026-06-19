package release

// ProcessInfo 结构体用于存储进程信息
type ProcessInfo struct {
	PID     int    // PID 表示进程的ID
	Port    int    // Port 表示进程占用的端口
	Name    string // Name 表示进程的名称
	CmdLine string // CmdLine 表示启动进程的命令行
}
