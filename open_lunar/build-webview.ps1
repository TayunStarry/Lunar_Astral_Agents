# 启用 CGO
$env:CGO_ENABLED=1

# 编译 webview 版本
go build -tags webview -ldflags="-s -w" -o ../Lunar-Astral-Agents.exe