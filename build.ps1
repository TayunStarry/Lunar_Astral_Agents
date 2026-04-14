# 启用CGO
$env:CGO_ENABLED=1

# 设置项目编译的基准目录
Set-Location -Path 'd:\Lunar_Astral_Agents'

# 跳转为构建目录
Write-Host 'Changing to build directory...'
Set-Location -Path './LunarCore'

# 构建客户端脚本
Write-Host 'Building client script (lunar)...'
npm run lunar
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Client script build failed, stopping execution' -ForegroundColor Red
    exit $LASTEXITCODE
}

# 构建工具包
Write-Host 'Building tool package (ltp)...'
npm run ltp
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Tool package build failed, stopping execution' -ForegroundColor Red
    exit $LASTEXITCODE
}

# 构建主程序
Write-Host 'Building main program...'
go build -tags webview -ldflags="-s -w" -o ../Lunar-Astral-Agents.exe
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Main program build failed, stopping execution' -ForegroundColor Red
    exit $LASTEXITCODE
}

Write-Host 'Build completed successfully!' -ForegroundColor Green

# 更改回原始目录
Set-Location -Path 'd:\Lunar_Astral_Agents'