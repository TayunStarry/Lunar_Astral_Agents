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

// ipCandidate 可选的本机 IP 地址候选（含所属网卡名）
type ipCandidate struct {
	IP        string // IPv4 地址
	Interface string // 所属网卡名称
}

// proxyInfo 代理服务运行态展示信息（TUI 信息面板与二维码生成用）
type proxyInfo struct {
	LocalURL string // 本地访问链接
	LANURL   string // 局域网访问链接（二维码内容）
	Target   string // 后端转发目标
	CertInfo string // 证书说明（加载来源等）
}
