# 编译图标
rsrc -ico icon.ico -o icon.syso

# 启用CGO
$env:CGO_ENABLED=1

# 编译客户端
# npm run lunar

# 编译LTP 1.0
# npm run ltp

# 构建可执行文件
go build -tags webview -ldflags="-s -w" -o ../Lunar-Astral-Agents.exe