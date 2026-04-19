# 编译图标
rsrc -ico icon.ico -o icon.syso

# 启用CGO
$env:CGO_ENABLED=1

# 构建可执行文件
go build -tags webview -ldflags="-s -w" -o ../../project_archiving.exe
