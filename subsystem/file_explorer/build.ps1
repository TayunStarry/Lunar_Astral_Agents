# 启用CGO
$env:CGO_ENABLED=1

# 构建可执行文件
go build -tags webview -ldflags="-s -w" -o ../../file_explorer.exe
