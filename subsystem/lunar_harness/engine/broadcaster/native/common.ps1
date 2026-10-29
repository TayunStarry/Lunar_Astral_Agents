# common.ps1 - broadcaster 原生 TTS 构建的共享工具函数
#
# 统一工具链探测、生成器选择、Vulkan SDK 探测、失效 CMake 缓存清理与
# 运行时依赖 DLL 就位逻辑。qwen / omnivoice 的构建都只通过本文件，避免
# 两套脚本各自维护一份同源判断。

$ErrorActionPreference = "Stop"

function Write-BuildLog {
    param([string]$Message, [string]$Color = "White")
    $line = "[{0}] {1}" -f (Get-Date -Format "HH:mm:ss"), $Message
    Write-Host $line -ForegroundColor $Color
    if ($script:BuildLogFile) {
        Add-Content -Path $script:BuildLogFile -Value $line
    }
}

function Test-CommandExists {
    param([string]$Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

# 执行原生命令并合并 stderr：cmake / 编译器写到 stderr 的警告在 Stop 语义下
# 会被升级为终止性错误，因此调用期间放宽 ErrorPreference；退出码由调用处判定。
function Invoke-NativeMerged {
    param([scriptblock]$Command)
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        return (& $Command 2>&1)
    }
    finally {
        $ErrorActionPreference = $previous
    }
}

function Get-CompilerInfo {
    $info = @{}
    $gcc = Get-Command "gcc" -ErrorAction SilentlyContinue
    $gpp = Get-Command "g++" -ErrorAction SilentlyContinue
    if ($gcc -and $gpp) {
        $info.GCC = $gcc.Source
        $info.GPP = $gpp.Source
        $info.Type = "MinGW"
    }
    else {
        $info.Type = "MSVC"
    }
    return $info
}

function Get-BestGenerator {
    param([string]$Requested)
    if ($Requested) { return $Requested }
    if (Test-CommandExists "gcc") { return "MinGW Makefiles" }
    if (Test-CommandExists "ninja") { return "Ninja" }
    if (Get-Command "MSBuild.exe" -ErrorAction SilentlyContinue) { return "Visual Studio 17 2022" }
    throw "未找到 C/C++ 工具链（需要 MinGW-w64 或 Visual Studio）"
}

function Find-VulkanSdk {
    if ($env:VULKAN_SDK -and (Test-Path $env:VULKAN_SDK)) { return $env:VULKAN_SDK }
    if ($env:VK_SDK_PATH -and (Test-Path $env:VK_SDK_PATH)) {
        $env:VULKAN_SDK = $env:VK_SDK_PATH
        return $env:VK_SDK_PATH
    }
    foreach ($root in @("C:\VulkanSDK", "${env:ProgramFiles}\VulkanSDK", "${env:LOCALAPPDATA}\VulkanSDK")) {
        if ($root -and (Test-Path $root)) {
            $latest = Get-ChildItem $root -Directory -ErrorAction SilentlyContinue |
                Sort-Object Name -Descending | Select-Object -First 1
            if ($latest) { return $latest.FullName }
            return $root
        }
    }
    if (Test-CommandExists "glslc") { return "PATH" }
    return ""
}

# 目录迁移后 CMakeCache 记录的绝对路径会失效，检测到不一致时自动清理。
function Reset-StaleCMakeCache {
    param([string]$BuildDir)
    $cacheFile = Join-Path $BuildDir "CMakeCache.txt"
    if (-not (Test-Path $cacheFile)) { return }
    foreach ($line in (Get-Content $cacheFile -TotalCount 20)) {
        if ($line -match "^#+ For build in directory: (.+)$") {
            $recorded = $Matches[1].Trim().Replace('/', '\')
            if ($recorded -ne $BuildDir) {
                Write-BuildLog "检测到失效 CMake 缓存（创建于 $recorded），清理后重新配置" "Yellow"
                Remove-Item -Recurse -Force $BuildDir
            }
            return
        }
    }
}

# 将引擎 DLL 的运行时依赖复制到模型目录（LOAD_WITH_ALTERED_SEARCH_PATH 从同目录解析）。
function Copy-RuntimeDependencies {
    param([string]$DllOutputDir)
    $gcc = Get-Command "gcc" -ErrorAction SilentlyContinue
    $gccBin = if ($gcc) { Split-Path -Parent $gcc.Source } else { "" }
    $vulkanSdk = Find-VulkanSdk
    $vulkanCandidates = @()
    if ($vulkanSdk -and $vulkanSdk -ne "PATH") {
        $vulkanCandidates += (Join-Path $vulkanSdk "Bin\vulkan-1.dll")
    }
    $vulkanCandidates += (Join-Path $env:SystemRoot "System32\vulkan-1.dll")
    $sources = @{
        "libgomp-1.dll"       = @((Join-Path $gccBin "libgomp-1.dll"))
        "libwinpthread-1.dll" = @((Join-Path $gccBin "libwinpthread-1.dll"))
        "vulkan-1.dll"        = $vulkanCandidates
    }
    foreach ($name in @("vulkan-1.dll", "libgomp-1.dll", "libwinpthread-1.dll")) {
        $dest = Join-Path $DllOutputDir $name
        if (Test-Path $dest) {
            Write-BuildLog "  [SKIP] $name（已存在）" "DarkGray"
            continue
        }
        $source = $sources[$name] | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
        if (-not $source) {
            Write-BuildLog "  [WARN] 未找到 $name 来源副本，运行时可能降级" "Yellow"
            continue
        }
        Copy-Item $source $dest -Force
        Write-BuildLog "  [OK] $name <- $source" "Green"
    }
}

# 从任意目录向上定位仓库根（subsystem\build_common.ps1 所在处）
function Find-RepoRoot {
    param([string]$StartDir)
    $cursor = $StartDir
    while ($cursor -and -not (Test-Path (Join-Path $cursor "subsystem\build_common.ps1"))) {
        $cursor = Split-Path -Parent $cursor
    }
    return $cursor
}
