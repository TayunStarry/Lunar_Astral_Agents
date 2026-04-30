# 编译图标
rsrc -ico icon.ico -o icon.syso

# 启用CGO
$env:CGO_ENABLED=1

# 编译 服务端脚本
npm run server.side

# 编译 客户端脚本
# npm run client.side

# 删除不必要的export
node removeExport.cjs

# 构建可执行文件
go build -tags webview -ldflags="-s -w" -o ../Lunar-Astral-Agents.exe