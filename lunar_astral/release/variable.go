package release

// commands 定义了用于查询指定端口范围 TCP 连接信息的 PowerShell 命令片段
var commands = []string{
	// 定义端口范围变量
	"$ports = %d..%d",
	// 获取指定端口的 TCP 连接并过滤
	"Get-NetTCPConnection -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort } |",
	// 选择本地端口和所属进程 ID 列
	"Select-Object LocalPort, OwningProcess |",
	// 转换为 CSV 格式，不包含类型信息
	"ConvertTo-Csv -NoTypeInformation",
}
