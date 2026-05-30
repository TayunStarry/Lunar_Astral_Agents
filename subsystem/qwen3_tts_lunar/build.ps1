# build.ps1 - Qwen3_TTS_Lunar Master Build Script
# 3-stage build: GGML -> C++ -> Go
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [switch]$Clean,

    [switch]$SkipGGML,

    [switch]$SkipCPP,

    [switch]$SkipGo,

    [switch]$EnableLog,

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    [switch]$EnableVulkan = $true,

    [string]$DllOutputDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

if ($EnableLog) {
    $BuildLogDir = Join-Path $ScriptDir "build_logs"
    if (-not (Test-Path $BuildLogDir)) {
        New-Item -ItemType Directory -Path $BuildLogDir -Force | Out-Null
    }

    $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $LogFile = Join-Path $BuildLogDir "build_${Timestamp}.log"
}

function Write-BuildLog {
    param([string]$Message, [string]$Color = "White")
    $timeStamp = Get-Date -Format "HH:mm:ss"
    $logMessage = "[$timeStamp] $Message"
    Write-Host $logMessage -ForegroundColor $Color
    if ($EnableLog) {
        Add-Content -Path $LogFile -Value $logMessage
    }
}

function Test-CommandExists {
    param([string]$Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

function Invoke-BuildScript {
    param(
        [string]$ScriptPath,
        [hashtable]$Arguments,
        [string]$StepName
    )

    $argList = @()
    foreach ($key in $Arguments.Keys) {
        $val = $Arguments[$key]
        if ($val -is [switch]) {
            if ($val) { $argList += "-$key" }
        } else {
            $argList += "-$key"
            $argList += "$val"
        }
    }

    Write-BuildLog ">> Calling: $ScriptPath $($argList -join ' ')" "DarkGray"

    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $ScriptPath @argList
    if ($LASTEXITCODE -ne 0) {
        throw "$StepName failed (exit code: $LASTEXITCODE)"
    }
}

function Build-IconIfNeeded {
    $iconPath = Join-Path $ScriptDir "icon.ico"
    $sysoPath = Join-Path $ScriptDir "icon.syso"

    if (-not (Test-Path $iconPath)) {
        return
    }
    if (Test-Path $sysoPath) {
        return
    }

    Write-BuildLog "[Icon] Compiling icon resource..." "Yellow"
    Push-Location $ScriptDir
    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        throw "rsrc icon compilation failed"
    }
    Pop-Location
    Write-BuildLog "  [OK] icon.syso generated" "Green"
}

Write-BuildLog "========================================" "Cyan"
Write-BuildLog "Qwen3_TTS_Lunar Build Start" "Cyan"
Write-BuildLog "========================================" "Cyan"
Write-BuildLog "Build Type: $BuildType" "White"
if ($EnableLog) {
    Write-BuildLog "Log File:   $LogFile" "White"
}
Write-BuildLog "DLL Out Dir: $DllOutputDir" "White"
if ($EnableVulkan) {
    Write-BuildLog "Vulkan GPU:  ENABLED" "Green"
} else {
    Write-BuildLog "Vulkan GPU:  DISABLED" "Yellow"
}
Write-BuildLog "========================================" "Cyan"

Write-BuildLog "[Check] Verifying build environment..." "Yellow"

if (Test-CommandExists "cmake") {
    $cmakeVer = & cmake --version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $cmakeVer" "Green"
} else {
    Write-BuildLog "  [FAIL] cmake not found" "Red"
    throw "cmake is required"
}

if (Test-CommandExists "go") {
    $goVer = & go version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $goVer" "Green"
} else {
    Write-BuildLog "  [FAIL] go not found" "Red"
    throw "go is required"
}

if (Test-CommandExists "gcc") {
    $gccVer = & gcc --version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $gccVer" "Green"
} else {
    Write-BuildLog "  [FAIL] gcc not found" "Red"
    throw "gcc is required (MinGW-w64 recommended)"
}

Write-BuildLog "[Check] Environment verification complete" "Green"

$goExeOutDir = Join-Path (Resolve-Path (Join-Path $ScriptDir "..\..")) ""
if (-not $DllOutputDir) {
    $DllOutputDir = $goExeOutDir
}
$DllOutputDir = [System.IO.Path]::GetFullPath($DllOutputDir)

$buildScriptArgs = @{
    BuildType = $BuildType
    Clean = $Clean
    ParallelJobs = $ParallelJobs
}

if ($OutputDir) {
    $buildScriptArgs.OutputDir = $OutputDir
}

if ($EnableLog) {
    $buildScriptArgs.EnableLog = $EnableLog
}

if ($EnableVulkan) {
    $buildScriptArgs.EnableVulkan = $EnableVulkan
}

if ($DllOutputDir) {
    $buildScriptArgs.DllOutputDir = $DllOutputDir
}

if (-not $SkipGGML) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 1/3 : Build GGML Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $ggmlScript = Join-Path $ScriptDir "build_ggml.ps1"
    Invoke-BuildScript -ScriptPath $ggmlScript -Arguments $buildScriptArgs -StepName "GGML Build"
} else {
    Write-BuildLog "[Stage 1/3] Skipped (-SkipGGML)" "Yellow"
}

if (-not $SkipCPP) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 2/3 : Build Qwen3-TTS C++ Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $cppScript = Join-Path $ScriptDir "build_cpp.ps1"
    Invoke-BuildScript -ScriptPath $cppScript -Arguments $buildScriptArgs -StepName "C++ Build"
} else {
    Write-BuildLog "[Stage 2/3] Skipped (-SkipCPP)" "Yellow"
}

if (-not $SkipGo) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 3/3 : Build Go Application" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    Build-IconIfNeeded

    Write-BuildLog "Running go build..." "Yellow"

    Push-Location $ScriptDir

    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    $goOutput = cmd /c "go build -v -o ..\..\Qwen3_TTS_Lunar.exe -ldflags ""-s -w"" 2>&1"
    $goExitCode = $LASTEXITCODE

    if ($goOutput -and $EnableLog) {
        foreach ($line in ($goOutput -split "`r`n")) {
            if ($line) {
                Add-Content -Path $LogFile -Value $line
            }
        }
    }

    if ($goExitCode -ne 0) {
        Write-BuildLog "Go build FAILED (exit code: $goExitCode)" "Red"
        if ($goOutput) { Write-Host $goOutput -ForegroundColor Red }
        Pop-Location
        throw "Go build failed"
    }

    $exePath = Join-Path $ScriptDir "Qwen3_TTS_Lunar.exe"
    if (Test-Path $exePath) {
        $exeSize = [math]::Round((Get-Item $exePath).Length / 1MB, 2)
        Write-BuildLog "  [OK] Qwen3_TTS_Lunar.exe ($exeSize MB)" "Green"
    }

    $targetDll = Join-Path $DllOutputDir "qwen3tts.dll"
    if (Test-Path $targetDll) {
        $dllSize = [math]::Round((Get-Item $targetDll).Length / 1MB, 2)
        Write-BuildLog "  [OK] qwen3tts.dll -> $targetDll ($dllSize MB)" "Green"
    } else {
        Write-BuildLog "  [WARN] qwen3tts.dll not found at $targetDll" "Yellow"
    }

    Pop-Location
    Write-BuildLog "Go build completed" "Green"
} else {
    Write-BuildLog "[Stage 3/3] Skipped (-SkipGo)" "Yellow"
}

Write-BuildLog "" "White"
Write-BuildLog "============================================================" "Green"
Write-BuildLog "  BUILD SUCCESSFUL!" "Green"
Write-BuildLog "============================================================" "Green"
if ($EnableLog) {
    Write-BuildLog "Log file: $LogFile" "White"
}
exit 0