# Studio WebView - Build Script
# 编译 Studio WebView 子系统，启用 WebView 窗口打开 studio-next.mosi.cn

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"
$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Check-GCCToolchain {
    Write-Host "[Check] MinGW-w64 GCC Compiler Toolchain..." -ForegroundColor Cyan

    if ($TargetOS -eq "windows") {
        if (-not (Test-CommandExists "gcc")) {
            throw "GCC compiler not found, CGO support requires GCC (install MinGW-w64 or TDM-GCC)"
        }

        $gccVersion = gcc --version 2>&1 | Select-Object -First 1
        Write-Host "  / $gccVersion" -ForegroundColor Green

        $gccTarget = gcc -dumpmachine 2>&1
        Write-Host "  / Target: $gccTarget" -ForegroundColor Green
    }
}

function Check-GoEnvironment {
    Write-Host "[Check] Go Environment..." -ForegroundColor Cyan

    if (-not (Test-CommandExists "go")) {
        throw "Go environment not found, install Go (https://golang.org/dl/)"
    }

    $goVersion = go version 2>&1
    Write-Host "  / $goVersion" -ForegroundColor Green
}

function Invoke-NativeCommand {
    param(
        [scriptblock]$ScriptBlock
    )

    $originalEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"

    try {
        & $ScriptBlock 2>&1 | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                Write-Host $_.Exception.Message -ForegroundColor DarkYellow
            } else {
                Write-Host $_
            }
        }
        return $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $originalEAP
    }
}

function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path (Join-Path $ScriptRoot "icon.ico"))) {
        return
    }

    if (Test-Path (Join-Path $ScriptRoot "icon.syso")) {
        return
    }

    Write-Host "  Building icon.syso from icon.ico..." -ForegroundColor Cyan
    Set-Location $ScriptRoot
    $exitCode = Invoke-NativeCommand { rsrc -ico icon.ico -o icon.syso }
    if ($exitCode -ne 0) { throw "rsrc icon compilation failed" }
    Set-Location $ScriptRoot
}

try {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host "  Studio WebView - Build System" -ForegroundColor Magenta
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host ""
    Write-Host "Target: $TargetOS / $TargetArch" -ForegroundColor Yellow
    Write-Host "Project: $ScriptRoot" -ForegroundColor Yellow
    Write-Host ""

    Write-Host "--- Stage 1: Environment Check ---" -ForegroundColor Yellow
    Check-GoEnvironment
    Check-GCCToolchain

    Write-Host ""
    Write-Host "--- Stage 2: Go Dependencies ---" -ForegroundColor Yellow

    $exitCode = Invoke-NativeCommand { go mod tidy }
    if ($exitCode -ne 0) {
        Write-Host "  ! go mod tidy had warnings, continuing build..." -ForegroundColor Yellow
    }

    Write-Host ""
    Write-Host "--- Stage 3: Compile Go Executable ---" -ForegroundColor Yellow

    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    if ($TargetOS -eq "windows") {
        $env:CC = "gcc"
        $env:CXX = "g++"
    }

    Build-IconIfNeeded

    $binaryName = "Studio_WebView.exe"
    if ($TargetOS -ne "windows") { $binaryName = "Studio_WebView" }
    $outputPath = Join-Path (Split-Path (Split-Path $ScriptRoot -Parent) -Parent) $binaryName

    $ldflags_arg = "-s -w -H windowsgui"
    if ($TargetOS -ne "windows") {
        $ldflags_arg = "-s -w"
    }

    Write-Host "  Output: $outputPath" -ForegroundColor Cyan
    Write-Host "  Flags: CGO_ENABLED=1 GOOS=$TargetOS GOARCH=$TargetArch" -ForegroundColor Cyan
    Write-Host ""

    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags_arg",
        "-trimpath",
        "-o", $outputPath
    )

    $exitCode = Invoke-NativeCommand { go $buildArgs }
    if ($exitCode -ne 0) { throw "Go build failed" }

    if (Test-Path $outputPath) {
        $fileInfo = Get-Item $outputPath
        $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
        Write-Host ""
        Write-Host "  / Studio_WebView build SUCCESS!" -ForegroundColor Green
        Write-Host "  / Output: $outputPath" -ForegroundColor Green
        Write-Host "  / Size: $sizeMB MB" -ForegroundColor Green
    } else {
        Write-Host "  ! WARNING: Output file not found" -ForegroundColor Red
    }

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  Studio WebView Build Complete" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
}
catch {
    Write-Host ""
    Write-Host "[ERROR] Studio WebView build failed: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
finally {
    Set-Location $ScriptRoot
}
