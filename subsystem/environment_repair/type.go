package main

// ProcessInfo 存储进程信息
type ProcessInfo struct {
	PID     int    // 进程ID
	Port    int    // 占用端口
	Name    string // 进程名称
	CmdLine string // 启动命令行
}

// MenuOption CLI 菜单选项
type MenuOption struct {
	Key         string // 按键
	Title       string // 标题
	Description string // 描述
	Action      func() // 执行函数
}