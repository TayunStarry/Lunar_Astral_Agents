package main

import (
	"embed"
	"sync"
)

// EmbeddedLocalData 嵌入的本地数据资源文件系统
// 包含 audios/、multimedia/background/、multimedia/placeholder/、multimedia/icon/ 以及 package/ 下的库资源
// （无 metadata.json 的子目录与裸露的 js 文件），用于启动时补全缺失的 local_data 内容。
// 资源由 build.ps1 的 Sync-EmbeddedData 在编译前从 ../local_data 同步到 embedded_data/。
//
//go:embed embedded_data/*
var EmbeddedLocalData embed.FS

// embeddedDataRoot 嵌入资源的根目录名（与 //go:embed embedded_data/* 对应）
const embeddedDataRoot = "embedded_data"

// 两字段配置屏（[2] 端口释放、[3] 代理端口配置）的字段索引，供 ↑/↓ 焦点切换复用
const (
	fieldPrimary   = iota // 起始端口 / 后端 HTTP 端口
	fieldSecondary        // 结束端口 / 代理 HTTPS 端口
	fieldCount            // 字段总数
)

// menuItemCount 主菜单条目数（4 个功能 + 日志查询 + 退出程序）
const menuItemCount = 6

// TUI 日志通道：TUI 激活时运行日志送入通道渲染，未激活时直接打印控制台
var (
	uiLogMu  sync.Mutex  // 保护 tuiLogCh 的并发读写
	tuiLogCh chan string // 非 nil 表示 TUI 已激活
)

// powershellCommands 用于查询指定端口范围 TCP 连接信息的 PowerShell 命令片段
var powershellCommands = []string{
	"$ports = %d..%d",
	"Get-NetTCPConnection -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort } |",
	"Select-Object LocalPort, OwningProcess |",
	"ConvertTo-Csv -NoTypeInformation",
}

// PortRange 端口扫描范围
type PortRange struct {
	Start int
	End   int
}
