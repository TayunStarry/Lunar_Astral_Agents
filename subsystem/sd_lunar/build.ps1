# SD Lunar - Build Script
# GCC toolchain configuration, Go module dependencies and project compilation

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

        if (-not (Test-CommandExists "g++")) {
            Write-Host "  ! g++ compiler not found, C++ compilation may be limited" -ForegroundColor Yellow
        } else {
            $gppVersion = g++ --version 2>&1 | Select-Object -First 1
            Write-Host "  / g++: $gppVersion" -ForegroundColor Green
        }
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

    Set-Location $ScriptRoot
    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc icon compilation failed" }
}

try {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host "  SD Lunar - Image Gen Test System Build" -ForegroundColor Magenta
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host ""
    Write-Host "Target: $TargetOS / $TargetArch" -ForegroundColor Yellow
    Write-Host "Project: $ScriptRoot" -ForegroundColor Yellow
    Write-Host ""

    Write-Host "--- Stage 1: Environment Check ---" -ForegroundColor Yellow
    Check-GoEnvironment
    Check-GCCToolchain

    Write-Host ""
    Write-Host "--- Stage 2: C++ Build (GGML + sd-cli) ---" -ForegroundColor Yellow

    Set-Location $ScriptRoot

    $ggmlBuildScript = Join-Path $ScriptRoot "build_ggml.ps1"
    if (Test-Path $ggmlBuildScript) {
        Write-Host "  Pre-building GGML library with Vulkan..." -ForegroundColor Cyan
        & powershell -ExecutionPolicy Bypass -File $ggmlBuildScript
        if ($LASTEXITCODE -ne 0) { throw "GGML pre-build failed" }
    } else {
        Write-Host "  ! build_ggml.ps1 not found, using existing build_ggml" -ForegroundColor Yellow
    }

    $cppBuildDir = Join-Path $ScriptRoot "cpp\build"
    $sdCliExe = Join-Path $cppBuildDir "bin\sd-cli.exe"

    if (-not (Test-Path $sdCliExe)) {
        Write-Host "  Building sd-cli..." -ForegroundColor Cyan
        New-Item -ItemType Directory -Path $cppBuildDir -Force | Out-Null

        $ggmlBuildDir = Join-Path $ScriptRoot "cpp\build_ggml"
        $cmakeArgs = @(
            "-S", (Join-Path $ScriptRoot "cpp"),
            "-B", $cppBuildDir,
            "-G", "MinGW Makefiles",
            "-DCMAKE_BUILD_TYPE=Release",
            "-DSD_VULKAN=ON",
            "-DGGML_BUILD_DIR=$ggmlBuildDir",
            "-DCMAKE_C_COMPILER=gcc",
            "-DCMAKE_CXX_COMPILER=g++"
        )
        $exitCode = Invoke-NativeCommand { cmake @cmakeArgs }
        if ($exitCode -ne 0) { throw "CMake configure failed" }

        $exitCode = Invoke-NativeCommand { mingw32-make -j4 -C $cppBuildDir sd-cli }
        if ($exitCode -ne 0) { throw "sd-cli build failed" }
    } else {
        Write-Host "  / sd-cli.exe already built, skipping" -ForegroundColor Green
    }

    if (Test-Path $sdCliExe) {
        $sdCliSize = [math]::Round((Get-Item $sdCliExe).Length / 1MB, 2)
        Write-Host "  / sd-cli.exe: $sdCliSize MB" -ForegroundColor Green
    }

    Write-Host ""
    Write-Host "--- Stage 3: Go Dependencies ---" -ForegroundColor Yellow

    $exitCode = Invoke-NativeCommand { go mod tidy }
    if ($exitCode -ne 0) {
        Write-Host "  ! go mod tidy had warnings, continuing build..." -ForegroundColor Yellow
    }

    Write-Host ""
    Write-Host "--- Stage 4: Compile Go Executable ---" -ForegroundColor Yellow

    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    if ($TargetOS -eq "windows") {
        $env:CC = "gcc"
        $env:CXX = "g++"
    }

    Build-IconIfNeeded

    $binaryName = "SD_Lunar.exe"
    if ($TargetOS -ne "windows") { $binaryName = "SD_Lunar" }
    $outputPath = Join-Path (Split-Path (Split-Path $ScriptRoot -Parent) -Parent) $binaryName

    $ldflags_arg = "-s -w"
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
        Write-Host "  / SD_Lunar build SUCCESS!" -ForegroundColor Green
        Write-Host "  / Output: $outputPath" -ForegroundColor Green
        Write-Host "  / Size: $sizeMB MB" -ForegroundColor Green
    } else {
        Write-Host "  ! WARNING: Output file not found" -ForegroundColor Red
    }

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  SD Lunar Build Complete" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
}
catch {
    Write-Host ""
    Write-Host "[ERROR] SD Lunar build failed: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
finally {
    Set-Location $ScriptRoot
}