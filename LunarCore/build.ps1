<#
.SYNOPSIS
    Luna_Astral 构建脚本
.DESCRIPTION
    安装 npm 依赖、处理 Go 模块、编译前端资源并构建 Go 可执行文件。
    支持独立编译，也可被主调度脚本调用（使用 -SkipCheck 跳过环境检查）。
.PARAMETER TargetOS
    目标操作系统，可选值：windows, linux, darwin。默认 windows。
.PARAMETER TargetArch
    目标 CPU 架构，可选值：amd64, arm64。默认 amd64。
.PARAMETER SkipCheck
    跳过环境与依赖检查（仅当确认环境已就绪时使用）。
.EXAMPLE
    .\build.ps1 -TargetOS linux -TargetArch arm64
.EXAMPLE
    .\build.ps1 -SkipCheck
#>

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",
    [switch]$SkipCheck
)

$ErrorActionPreference = "Stop"

# ---------- 环境检查（仅在独立编译时执行）----------
function Check-GoEnvironment {
    Write-Host "[Env] 检查 Go 环境..." -ForegroundColor Cyan
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        throw "未找到 Go。请安装 Go 1.20 或更高版本：https://go.dev/dl/"
    }
    $goVersion = (go version | Select-String -Pattern "go(\d+\.\d+\.\d+)" | ForEach-Object { $_.Matches.Groups[1].Value })
    if (-not $goVersion) {
        throw "无法解析 Go 版本。请确保已安装 Go 1.20+。"
    }
    $minVersion = [version]"1.20.0"
    if ([version]$goVersion -lt $minVersion) {
        throw "Go 版本过低 ($goVersion)，需要 >= 1.20。请升级：https://go.dev/dl/"
    }
    Write-Host "  ✓ Go $goVersion" -ForegroundColor Green
}

function Check-GCC {
    Write-Host "[Env] 检查 GCC (CGO 需要)..." -ForegroundColor Cyan
    $gccCmd = Get-Command gcc -ErrorAction SilentlyContinue
    if (-not $gccCmd) {
        throw "未找到 GCC。CGO 需要 GCC，Windows 下请安装 MinGW-w64 或 TDM-GCC：https://www.mingw-w64.org/"
    }
    $gccOutput = & gcc --version 2>&1 | Select-Object -First 1
    Write-Host "  ✓ GCC: $gccOutput" -ForegroundColor Green
}

function Check-NodeEnvironment {
    Write-Host "[Env] 检查 Node.js..." -ForegroundColor Cyan
    $nodeCmd = Get-Command node -ErrorAction SilentlyContinue
    if (-not $nodeCmd) {
        throw "未找到 Node.js。请安装 Node.js 18+：https://nodejs.org/"
    }
    $nodeVersion = & node --version
    Write-Host "  ✓ Node.js $nodeVersion" -ForegroundColor Green
}

function Check-NPM {
    Write-Host "[Env] 检查 npm..." -ForegroundColor Cyan
    $npmCmd = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npmCmd) {
        throw "未找到 npm（通常随 Node.js 安装）。"
    }
    $npmVersion = & npm --version
    Write-Host "  ✓ npm v$npmVersion" -ForegroundColor Green
}

# ---------- 依赖管理 ----------
function Invoke-NpmInstall {
    Write-Host "[Deps] 安装 npm 依赖..." -ForegroundColor Cyan
    if (-not (Test-Path "package.json")) {
        Write-Warning "未找到 package.json，跳过 npm install。"
        return
    }
    try {
        & npm install 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "npm install 失败。" }
    }
    catch {
        throw "npm 依赖安装失败: $_。请检查网络与 package.json。"
    }
    Write-Host "  ✓ npm 依赖就绪" -ForegroundColor Green
}

function Invoke-GoDependencies {
    Write-Host "[Deps] 下载/验证 Go 模块..." -ForegroundColor Cyan
    if (-not (Test-Path "go.mod")) {
        throw "未找到 go.mod，请在项目根目录运行此脚本。"
    }
    try {
        & go mod download 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "go mod download 返回非零退出码（常见于本地模块无哈希），已忽略。"
        }
        Write-Host "  ✓ 已跳过 go mod verify（本地模块仅确认路径存在）" -ForegroundColor DarkGray
    }
    catch {
        throw "Go 模块依赖处理失败: $_。请检查网络或运行 'go mod tidy'。"
    }
    Write-Host "  ✓ Go 模块依赖就绪" -ForegroundColor Green
}

function Test-LocalReplacements {
    Write-Host "[Deps] 检查 go.mod 中的本地替换路径..." -ForegroundColor Cyan
    if (-not (Test-Path "go.mod")) { return }

    $modContent = Get-Content "go.mod" -Raw
    $pattern = 'replace\s+\S+\s+=>\s+(\.\.?[\\/][^\s]+|[a-zA-Z]:[\\/][^\s]+|/[^\s]+)'
    $matches = [regex]::Matches($modContent, $pattern)

    foreach ($m in $matches) {
        $localPath = $m.Groups[1].Value
        $modDir = Split-Path -Path (Resolve-Path "go.mod") -Parent
        $fullPath = [System.IO.Path]::GetFullPath([System.IO.Path]::Combine($modDir, $localPath))
        if (-not (Test-Path $fullPath)) {
            throw "本地模块路径不存在: '$localPath' (解析为: $fullPath)。请检查 go.mod 的 replace 指令，或执行 'go mod tidy'。"
        }
    }
    Write-Host "  ✓ 所有本地替换路径均存在" -ForegroundColor Green
}

function Invoke-RsrcTool {
    $rsrcCmd = Get-Command rsrc -ErrorAction SilentlyContinue
    if (-not $rsrcCmd) {
        Write-Host "[Deps] rsrc 未安装，自动安装..." -ForegroundColor Yellow
        try {
            & go install github.com/akavel/rsrc@latest 2>&1 | Out-Host
            if ($LASTEXITCODE -ne 0) { throw }
            $goPath = & go env GOPATH
            $env:Path = "$goPath\bin;$env:Path"
        }
        catch {
            throw "安装 rsrc 失败: $_。请手动执行：go install github.com/akavel/rsrc@latest"
        }
        $rsrcCmd = Get-Command rsrc -ErrorAction SilentlyContinue
        if (-not $rsrcCmd) {
            throw "安装 rsrc 后仍未找到命令，请确保 %GOPATH%\bin 在 PATH 中。"
        }
    }
    Write-Host "  ✓ rsrc 工具可用" -ForegroundColor Green
}

# ---------- 构建流程 ----------
function Build-LunaAstral {
    Write-Host "`n========== 开始构建 Luna_Astral ==========" -ForegroundColor Magenta
    Write-Host "目标平台: $TargetOS / $TargetArch" -ForegroundColor Magenta

    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    if ($TargetOS -eq "windows" -and (Test-Path "icon.ico")) {
        if (Test-Path "icon.syso") {
            Write-Host "[Icon] icon.syso 已存在，跳过图标编译。" -ForegroundColor DarkGray
        }
        else {
            Write-Host "[Icon] 编译图标资源..." -ForegroundColor Cyan
            Invoke-RsrcTool
            & rsrc -ico icon.ico -o icon.syso
            if ($LASTEXITCODE -ne 0) { throw "rsrc 执行失败。" }
        }
    }
    elseif ($TargetOS -ne "windows") {
        Write-Host "[Icon] 非 Windows 目标，跳过图标打包。" -ForegroundColor DarkGray
    }

    Write-Host "[PreBuild] 编译服务端脚本..." -ForegroundColor Cyan
    $originalErrorAction = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    & npm run server.side 2>&1 | Where-Object { $_ -isnot [System.Management.Automation.ErrorRecord] } | Out-Host
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = $originalErrorAction
    if ($exitCode -ne 0) { throw "npm run server.side 失败。" }

    Write-Host "[PreBuild] 执行 removeExport.cjs..." -ForegroundColor Cyan
    if (Test-Path "removeExport.cjs") {
        & node removeExport.cjs 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "removeExport.cjs 执行失败。" }
    }
    else {
        Write-Warning "removeExport.cjs 未找到，跳过。"
    }

    $ldflags = "-s -w"
    if ($TargetOS -eq "windows") {
       # $ldflags += " -H windowsgui"
    }

    $binaryName = "Luna_Astral"
    if ($TargetOS -eq "windows") { $binaryName += ".exe" }
    $outputPath = "..\$binaryName"

    Write-Host "[Build] 编译 Go 二进制..." -ForegroundColor Cyan
    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败。" }

    Write-Host "✓ 构建成功: $outputPath" -ForegroundColor Green
    Write-Host "========== 完成 ==========`n" -ForegroundColor Magenta
}

# ---------- 主流程 ----------
try {
    if (-not $SkipCheck) {
        Write-Host "[LunarCore] 独立编译模式，执行完整环境检查..." -ForegroundColor Cyan
        Check-GoEnvironment
        Check-GCC
        Check-NodeEnvironment
        Check-NPM
    }
    else {
        Write-Host "[LunarCore] 由主调度脚本调用，跳过环境检查。" -ForegroundColor Yellow
    }
    Invoke-NpmInstall
    Invoke-GoDependencies
    Test-LocalReplacements
    Build-LunaAstral
}
catch {
    Write-Host "`n[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}
