# LunarTick 构建脚本
$ErrorActionPreference = "Stop"

$projectDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $projectDir

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  LunarTick 通用程序执行引擎 v5.0" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$exeDir = Join-Path $projectDir "build"
if (-not (Test-Path $exeDir)) {
    New-Item -ItemType Directory -Path $exeDir | Out-Null
}

Write-Host "[构建] 编译主程序..." -ForegroundColor Green
$mainPath = Join-Path $projectDir "cmd" "lunartick" "main.go"
$outputPath = Join-Path $exeDir "lunartick.exe"

go build -ldflags "-s -w" -o $outputPath $mainPath

if ($LASTEXITCODE -eq 0) {
    Write-Host "[成功] 构建完成: $outputPath" -ForegroundColor Green
} else {
    Write-Host "[错误] 构建失败" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "[信息] 使用方法:" -ForegroundColor Yellow
Write-Host "  $outputPath --api-port=36800 --tick-ms=100" -ForegroundColor White
Write-Host "  $outputPath --developer --load=script.md" -ForegroundColor White