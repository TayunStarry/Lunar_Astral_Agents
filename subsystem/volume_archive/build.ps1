param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    [switch]$SkipCheck
)

$separator = "=" * 40

Write-Host $separator -ForegroundColor Cyan
Write-Host "项目打包工具构建脚本" -ForegroundColor Cyan
Write-Host $separator -ForegroundColor Cyan
Write-Host ""

if (-not $SkipCheck) {
    Write-Host "检查环境..." -ForegroundColor Yellow

    $goVersion = go version
    if ($LASTEXITCODE -ne 0) {
        Write-Host "错误: 未找到 Go 编译器" -ForegroundColor Red
        exit 1
    }
    Write-Host "Go 版本: $goVersion" -ForegroundColor Green

    if ($TargetOS -eq "windows") {
        Write-Host "检查 7-Zip..." -ForegroundColor Yellow
        $7zPaths = @(
            ".\local_data\package\archive\7z.exe",
            "C:\Program Files\7-Zip\7z.exe",
            "C:\Program Files (x86)\7-Zip\7z.exe"
        )

        $7zFound = $false
        foreach ($path in $7zPaths) {
            if (Test-Path $path) {
                Write-Host "7-Zip 已找到: $path" -ForegroundColor Green
                $7zFound = $true
                break
            }
        }

        if (-not $7zFound) {
            Write-Host "警告: 未找到 7-Zip，但构建仍将继续" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "配置构建参数:" -ForegroundColor Yellow
Write-Host "  目标系统: $TargetOS" -ForegroundColor White
Write-Host "  目标架构: $TargetArch" -ForegroundColor White
Write-Host ""

$env:GOOS = $TargetOS
$env:GOARCH = $TargetArch
$env:CGO_ENABLED = 0

$ldflags = "-s -w"
$outputName = "volume_archive.exe"
if ($TargetOS -ne "windows") {
    $outputName = "volume_archive"
}

Write-Host "开始编译..." -ForegroundColor Yellow
go build -ldflags $ldflags -o $outputName .

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host $separator -ForegroundColor Green
    Write-Host "构建成功！" -ForegroundColor Green
    Write-Host "输出文件: .\$outputName" -ForegroundColor Green
    Write-Host $separator -ForegroundColor Green

    $fileInfo = Get-Item $outputName
    $sizeMB = [math]::Round($fileInfo.Length / 1048576, 2)
    Write-Host "文件大小: $sizeMB MB" -ForegroundColor White
} else {
    Write-Host ""
    Write-Host $separator -ForegroundColor Red
    Write-Host "构建失败！" -ForegroundColor Red
    Write-Host $separator -ForegroundColor Red
    exit 1
}
