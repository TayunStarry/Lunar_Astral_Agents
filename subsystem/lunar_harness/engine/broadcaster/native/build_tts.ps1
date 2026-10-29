# build_tts.ps1 - broadcaster 原生 TTS 构建统一入口
#
# 目录约定：
#   broadcaster/qwen/cpp         Qwen3-TTS C++ 源码 + 内置 ggml 0.11.1
#   broadcaster/omnivoice/cpp    OmniVoice C++ 源码 + 内置 ggml 0.26.0（superbuild）
#   broadcaster/native/build/    全部构建产物（gitignored）
#     ├─ ggml/qwen/              Qwen 使用的独立 ggml 静态库
#     ├─ qwen/                   qwen3tts.dll 中间产物
#     └─ omnivoice/              omnivoice.dll 中间产物
#
# ggml 合并结论：两个引擎内置的 ggml 版本分别为 0.11.1 与 0.26.0，源码与 API
# 不兼容，不能共用同一份静态库；因此保留各自源码树，仅统一构建入口、参数与
# 输出路径。OmniVoice 的 ggml 由其 CMake superbuild 一并构建，Qwen 的 ggml
# 由本脚本的第一阶段独立预编译。
param(
    [ValidateSet("qwen", "omnivoice", "kokoro", "all")]
    [string]$Engine = "all",

    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [switch]$Clean,
    [switch]$SkipGGML,
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

$ErrorActionPreference = "Stop"
$NativeDir = $PSScriptRoot
. (Join-Path $NativeDir "common.ps1")

$repoRoot = Find-RepoRoot -StartDir $NativeDir
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }

if ($EnableLog) {
    $logDir = Join-Path $NativeDir "build_logs"
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    $script:BuildLogFile = Join-Path $logDir ("tts_{0}_{1}.log" -f $Engine, (Get-Date -Format "yyyyMMdd_HHmmss"))
}

if ($TargetOS -ne "windows") {
    Write-BuildLog "[SKIP] TargetOS=$TargetOS : TTS 原生库仅产出 Windows/MinGW DLL" "Yellow"
    exit 0
}
if ($ParallelJobs -le 0) { $ParallelJobs = 4 }

function Get-EngineSpec {
    param([string]$Name)
    $broadcaster = Split-Path -Parent $NativeDir
    switch ($Name) {
        "qwen" {
            return @{
                Name        = "qwen"
                Label       = "Qwen3-TTS"
                CppDir      = Join-Path $broadcaster "qwen\cpp"
                GGMLDir     = Join-Path $broadcaster "qwen\cpp\ggml"
                GGMLBuild   = Join-Path $NativeDir "build\ggml\qwen"
                BuildDir    = Join-Path $NativeDir "build\qwen"
                DllName     = "qwen3tts.dll"
                ModelDir    = "Qwen3-TTS"
                BuildGGML   = $true
            }
        }
        "omnivoice" {
            return @{
                Name        = "omnivoice"
                Label       = "OmniVoice"
                CppDir      = Join-Path $broadcaster "omnivoice\cpp"
                GGMLDir     = Join-Path $broadcaster "omnivoice\cpp\ggml"
                GGMLBuild   = Join-Path $NativeDir "build\omnivoice"
                BuildDir    = Join-Path $NativeDir "build\omnivoice"
                DllName     = "omnivoice.dll"
                ModelDir    = "OmniVoice"
                BuildGGML   = $false
            }
        }
        default { throw "未知原生 TTS 引擎: $Name" }
    }
}

function Invoke-CMakeStep {
    param([string]$Step, [string[]]$Arguments)
    Write-BuildLog ">> cmake $Step" "Yellow"
    $output = Invoke-NativeMerged { & cmake @Arguments }
    $code = $LASTEXITCODE
    if ($code -ne 0) {
        $tail = @($output)
        $start = [Math]::Max(0, $tail.Count - 40)
        for ($i = $start; $i -lt $tail.Count; $i++) { Write-Host ("  " + $tail[$i]) -ForegroundColor DarkRed }
        throw "CMake $Step 失败 (exit code: $code)"
    }
    Write-BuildLog "CMake $Step 完成" "Green"
}

function Build-QwenGGML {
    param($Spec)
    $bt = $BuildType.ToUpper()
    $generator = Get-BestGenerator -Requested $Generator
    $compiler = Get-CompilerInfo
    if ($Clean -and (Test-Path $Spec.GGMLBuild)) { Remove-Item -Recurse -Force $Spec.GGMLBuild }
    New-Item -ItemType Directory -Path $Spec.GGMLBuild -Force | Out-Null
    Reset-StaleCMakeCache -BuildDir $Spec.GGMLBuild

    $vulkan = $false
    if ($EnableVulkan) { $vulkan = [bool](Find-VulkanSdk) }

    $args = @(
        "-S", $Spec.GGMLDir,
        "-B", $Spec.GGMLBuild,
        "-G", $generator,
        "-DCMAKE_BUILD_TYPE=$bt",
        "-DBUILD_SHARED_LIBS=OFF",
        "-DGGML_STATIC=ON",
        "-DGGML_BUILD_TESTS=OFF",
        "-DGGML_BUILD_EXAMPLES=OFF",
        "-DGGML_NATIVE=ON",
        "-DGGML_OPENMP=ON",
        "-DGGML_CUDA=OFF",
        "-DGGML_METAL=OFF",
        "-DGGML_BLAS=OFF",
        "-DGGML_BACKEND_DL=OFF",
        "-DGGML_CPU_ALL_VARIANTS=OFF",
        "-DGGML_CPU_KLEIDIAI=OFF",
        "-DGGML_LLAMAFILE=OFF",
        "-DGGML_CPU_HBM=OFF",
        "-DGGML_LTO=OFF",
        "-DGGML_CCACHE=OFF"
    )
    $args += ("-DGGML_VULKAN=" + $(if ($vulkan) { "ON" } else { "OFF" }))
    if ($compiler.Type -eq "MinGW") {
        $args += "-DCMAKE_C_COMPILER=$($compiler.GCC)"
        $args += "-DCMAKE_CXX_COMPILER=$($compiler.GPP)"
    }
    Invoke-CMakeStep -Step "configure ggml(qwen)" -Arguments $args
    Invoke-CMakeStep -Step "build ggml(qwen)" -Arguments @("--build", $Spec.GGMLBuild, "--config", $bt, "--parallel", $ParallelJobs)

    foreach ($lib in @("src\ggml.a", "src\ggml-base.a", "src\ggml-cpu.a")) {
        $path = Join-Path $Spec.GGMLBuild $lib
        if (-not (Test-Path $path)) { throw "GGML 产物缺失: $path" }
        Write-BuildLog "  [OK] $path" "Green"
    }
}

function Build-QwenEngine {
    param($Spec)
    $bt = $BuildType.ToUpper()
    $generator = Get-BestGenerator -Requested $Generator
    $compiler = Get-CompilerInfo
    $buildDir = if ($OutputDir) { $OutputDir } else { $Spec.BuildDir }
    $dllOut = if ($DllOutputDir) { $DllOutputDir } else { Join-Path $repoRoot ("local_data\models\" + $Spec.ModelDir) }
    New-Item -ItemType Directory -Path $dllOut -Force | Out-Null
    if ($Clean -and (Test-Path $buildDir)) { Remove-Item -Recurse -Force $buildDir }
    New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
    Reset-StaleCMakeCache -BuildDir $buildDir

    if (-not $SkipGGML) {
        foreach ($lib in @("src\ggml.a", "src\ggml-base.a", "src\ggml-cpu.a")) {
            if (-not (Test-Path (Join-Path $Spec.GGMLBuild $lib))) {
                throw "GGML 静态库缺失，请先执行 build_tts.ps1 -Engine qwen（或去掉 -SkipGGML）"
            }
        }
    }

    $args = @(
        "-S", $Spec.CppDir,
        "-B", $buildDir,
        "-G", $generator,
        "-DCMAKE_BUILD_TYPE=$bt",
        "-DQWEN3_TTS_TIMING=OFF",
        "-DGGML_BUILD_DIR=$($Spec.GGMLBuild)",
        "-DGGML_LIB_DIR=$(Join-Path $Spec.GGMLBuild 'src')",
        "-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$dllOut"
    )
    if ($compiler.Type -eq "MinGW") {
        $args += "-DCMAKE_C_COMPILER=$($compiler.GCC)"
        $args += "-DCMAKE_CXX_COMPILER=$($compiler.GPP)"
    }
    Invoke-CMakeStep -Step "configure qwen3tts" -Arguments $args
    Invoke-CMakeStep -Step "build qwen3tts" -Arguments @("--build", $buildDir, "--config", $bt, "--parallel", $ParallelJobs)
    Install-EngineArtifacts -Spec $Spec -BuildDir $buildDir -DllOutputDir $dllOut
}

function Build-OmniVoiceEngine {
    param($Spec)
    $bt = $BuildType.ToUpper()
    $generator = Get-BestGenerator -Requested $Generator
    $compiler = Get-CompilerInfo
    $buildDir = if ($OutputDir) { $OutputDir } else { $Spec.BuildDir }
    $dllOut = if ($DllOutputDir) { $DllOutputDir } else { Join-Path $repoRoot ("local_data\models\" + $Spec.ModelDir) }
    New-Item -ItemType Directory -Path $dllOut -Force | Out-Null
    if ($Clean -and (Test-Path $buildDir)) { Remove-Item -Recurse -Force $buildDir }
    New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
    Reset-StaleCMakeCache -BuildDir $buildDir

    $vulkan = if ($EnableVulkan -and (Find-VulkanSdk)) { "ON" } else { "OFF" }
    $args = @(
        "-S", $Spec.CppDir,
        "-B", $buildDir,
        "-G", $generator,
        "-DCMAKE_BUILD_TYPE=$bt",
        "-DGGML_VULKAN=$vulkan",
        "-DOMNIVOICE_SHARED=ON",
        "-DGGML_CUDA=OFF",
        "-DGGML_METAL=OFF",
        "-DGGML_BLAS=OFF",
        "-DGGML_BACKEND_DL=OFF",
        "-DGGML_NATIVE=ON",
        "-DCMAKE_SHARED_LINKER_FLAGS=-static-libgcc -static-libstdc++ -Wl,-Bstatic -lwinpthread -Wl,-Bdynamic",
        "-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$buildDir"
    )
    if ($compiler.Type -eq "MinGW") {
        $args += "-DCMAKE_C_COMPILER=$($compiler.GCC)"
        $args += "-DCMAKE_CXX_COMPILER=$($compiler.GPP)"
    }
    Invoke-CMakeStep -Step "configure omnivoice" -Arguments $args
    Invoke-CMakeStep -Step "build omnivoice" -Arguments @("--build", $buildDir, "--target", "omnivoice", "--config", $bt, "--parallel", $ParallelJobs)
    Install-EngineArtifacts -Spec $Spec -BuildDir $buildDir -DllOutputDir $dllOut
}

function Install-EngineArtifacts {
    param($Spec, [string]$BuildDir, [string]$DllOutputDir)
    $target = Join-Path $DllOutputDir $Spec.DllName
    if (-not (Test-Path $target)) {
        $candidates = @(
            (Join-Path $BuildDir $Spec.DllName),
            (Join-Path $BuildDir ("lib" + $Spec.DllName))
        )
        foreach ($candidate in $candidates) {
            if (Test-Path $candidate) {
                Copy-Item $candidate $target -Force
                break
            }
        }
    }
    if (-not (Test-Path $target)) { throw "未找到引擎 DLL: $($Spec.DllName)（构建目录 $BuildDir）" }
    $size = [math]::Round((Get-Item $target).Length / 1MB, 2)
    Write-BuildLog "[OK] $($Spec.DllName) -> $target ($size MB)" "Green"
    Copy-RuntimeDependencies -DllOutputDir $DllOutputDir
}

$engines = switch ($Engine) {
    "all" { @("qwen", "omnivoice") }
    "kokoro" { @() }
    default { @($Engine) }
}
if ($Engine -eq "kokoro") {
    Write-BuildLog "Kokoro 为纯 Go + ONNX Runtime 引擎，无原生 C++ 构建步骤" "Yellow"
    exit 0
}

Write-BuildLog "========================================" "Cyan"
Write-BuildLog "Broadcaster 原生 TTS 构建：$($engines -join ', ') / $BuildType / $TargetArch" "Cyan"
Write-BuildLog "构建根目录：$NativeDir\build" "Cyan"
Write-BuildLog "========================================" "Cyan"

foreach ($name in $engines) {
    $spec = Get-EngineSpec -Name $name
    Write-BuildLog "" "White"
    Write-BuildLog "---- $($spec.Label) ----" "Cyan"
    if ($spec.BuildGGML -and -not $SkipGGML) {
        Build-QwenGGML -Spec $spec
    }
    elseif ($spec.BuildGGML -and $SkipGGML) {
        Write-BuildLog "已跳过 GGML 预编译（-SkipGGML）" "Yellow"
    }
    if ($name -eq "qwen") {
        Build-QwenEngine -Spec $spec
    }
    else {
        Build-OmniVoiceEngine -Spec $spec
    }
}

Write-BuildLog "========================================" "Green"
Write-BuildLog "原生 TTS 构建完成" "Green"
Write-BuildLog "========================================" "Green"
exit 0
