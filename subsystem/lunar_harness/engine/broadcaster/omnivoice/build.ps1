# omnivoice/build.ps1 - OmniVoice 构建兼容入口
# 实际构建逻辑统一由 ../native/build_tts.ps1 提供（CMake superbuild 内含 ggml）。
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",
    [switch]$Clean,
    [switch]$EnableLog,
    [bool]$EnableVulkan = $true,
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",
    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,
    [string]$Generator = "",
    [string]$OutputDir = "",
    [string]$DllOutputDir = ""
)

$shared = Join-Path (Split-Path -Parent $PSScriptRoot) "native\build_tts.ps1"
& $shared -Engine omnivoice -BuildType $BuildType -Clean:$Clean `
    -EnableLog:$EnableLog -EnableVulkan $EnableVulkan -TargetOS $TargetOS -TargetArch $TargetArch `
    -ParallelJobs $ParallelJobs -Generator $Generator -OutputDir $OutputDir -DllOutputDir $DllOutputDir
exit $LASTEXITCODE
