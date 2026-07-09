<#
.SYNOPSIS
    引擎层升级脚本 - 从 lunar.studio.integrated 拷贝引擎文件到 lunar_astral 客户端目录
.DESCRIPTION
    1. 创建 vendor/ 目录，拷贝 three.module.js 和 cannon-es.module.js
    2. 用源引擎 core/ 目录覆盖目标 core/ 目录（新增物理、相机、图元模块）
    3. 拷贝源引擎 engine.js 并适配导入路径（../vendor -> ./vendor, ../core -> ./core）
    4. 适配 BroadcastChannel 频道名（integrated-studio-bus -> lunar-astral-renderer）
.NOTES
    源目录: d:\Lunar_Astral_Agents\local_data\package\lunar.studio.integrated
    目标目录: d:\Lunar_Astral_Agents\lunar_astral\hierarchy\assets\client
    备份位置: 目标目录\engine_backup
#>

param(
    [string]$Source = "d:\Lunar_Astral_Agents\local_data\package\lunar.studio.integrated",
    [string]$Target = "d:\Lunar_Astral_Agents\lunar_astral\hierarchy\assets\client"
)

$ErrorActionPreference = "Stop"

# ==== 1. 创建 vendor/ 目录并拷贝第三方库 ====
Write-Host "[Step 1] 创建 vendor/ 目录并拷贝第三方库" -ForegroundColor Cyan
$vendorDir = Join-Path $Target "vendor"
New-Item -ItemType Directory -Path $vendorDir -Force | Out-Null

$vendorFiles = @("three.module.js", "cannon-es.module.js")
foreach ($f in $vendorFiles) {
    $src = Join-Path $Source "vendor\$f"
    $dst = Join-Path $vendorDir $f
    if (Test-Path $src) {
        Copy-Item $src $dst -Force
        Write-Host "  [OK] vendor/$f" -ForegroundColor Green
    } else {
        Write-Host "  [!] 源文件不存在: vendor/$f" -ForegroundColor Yellow
    }
}

# ==== 2. 拷贝 core/ 目录 ====
Write-Host "[Step 2] 拷贝 core/ 目录" -ForegroundColor Cyan
$srcCore = Join-Path $Source "core"
$dstCore = Join-Path $Target "core"
if (Test-Path $srcCore) {
    Get-ChildItem $srcCore -Filter "*.js" | ForEach-Object {
        Copy-Item $_.FullName $dstCore -Force
        Write-Host "  [OK] core/$($_.Name)" -ForegroundColor Green
    }
} else {
    Write-Host "  [!] 源 core/ 目录不存在" -ForegroundColor Yellow
}

# ==== 3. 拷贝 engine.js 并适配导入路径 ====
Write-Host "[Step 3] 拷贝 engine.js 并适配导入路径" -ForegroundColor Cyan
$srcEngine = Join-Path $Source "engine\engine.js"
$dstEngine = Join-Path $Target "engine.js"

if (Test-Path $srcEngine) {
    $content = Get-Content $srcEngine -Raw -Encoding UTF8

    # 适配导入路径
    $content = $content.Replace('../vendor/', './vendor/')
    $content = $content.Replace('../core/', './core/')
    # 适配 BroadcastChannel 频道名
    $content = $content.Replace('integrated-studio-bus', 'lunar-astral-renderer')

    Set-Content $dstEngine -Value $content -Encoding UTF8 -NoNewline
    Write-Host "  [OK] engine.js 已拷贝并适配路径" -ForegroundColor Green
} else {
    Write-Host "  [!] 源 engine.js 不存在" -ForegroundColor Yellow
}

# ==== 4. 验证拷贝结果 ====
Write-Host "[Step 4] 验证拷贝结果" -ForegroundColor Cyan

$expectedCore = @(
    "animation-runtime.js", "anim-group-runtime.js", "animation-codec.js",
    "animation-classifier.js", "body-rotation-interpreter.js", "controller-codec.js",
    "controller-vm.js", "geometry-loader.js", "interpolator.js", "keyframe.js",
    "material-system.js", "molang-runtime.js", "movement-controller.js",
    "outliner.js", "renderer.js", "special-animation-runtime.js", "texture-manager.js",
    "camera-controller.js", "character-physics.js", "physics-manager.js", "primitives.js"
)

$missing = @()
foreach ($f in $expectedCore) {
    $path = Join-Path $dstCore $f
    if (-not (Test-Path $path)) {
        $missing += $f
    }
}

if ($missing.Count -eq 0) {
    Write-Host "  [OK] 所有 $($expectedCore.Count) 个 core 模块验证通过" -ForegroundColor Green
} else {
    Write-Host "  [!] 缺失模块: $($missing -join ', ')" -ForegroundColor Yellow
}

foreach ($f in $vendorFiles) {
    $path = Join-Path $vendorDir $f
    if (Test-Path $path) {
        Write-Host "  [OK] vendor/$f 验证通过" -ForegroundColor Green
    } else {
        Write-Host "  [!] vendor/$f 缺失" -ForegroundColor Yellow
    }
}

if (Test-Path $dstEngine) {
    Write-Host "  [OK] engine.js 验证通过" -ForegroundColor Green
} else {
    Write-Host "  [!] engine.js 缺失" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "引擎升级完成！" -ForegroundColor Green
Write-Host "  源: $Source"
Write-Host "  目标: $Target"
Write-Host "  备份: $(Join-Path $Target 'engine_backup')"
