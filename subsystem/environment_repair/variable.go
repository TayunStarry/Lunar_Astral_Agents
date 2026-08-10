package main

import "embed"

// EmbeddedLocalData 嵌入的本地数据资源文件系统
// 包含 audios/、images/background/、images/placeholder/ 以及 package/ 下的库资源
// （无 metadata.json 的子目录与裸露的 js 文件），用于启动时补全缺失的 local_data 内容。
// 资源由 build.ps1 的 Sync-EmbeddedData 在编译前从 ../local_data 同步到 embedded_data/。
//
//go:embed embedded_data/*
var EmbeddedLocalData embed.FS

// embeddedDataRoot 嵌入资源的根目录名（与 //go:embed embedded_data/* 对应）
const embeddedDataRoot = "embedded_data"

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
