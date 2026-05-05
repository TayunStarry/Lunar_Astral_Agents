param (
    [string]$OriginalScriptPath,
    [string]$LaunchCommandLine,
    [switch]$Help,
    [string]$CorePrefix,
    [switch]$DisableUpdate,
    [switch]$BuildMode,
    [switch]$DisableProxy,
    [string]$UseCustomProxy,
    [switch]$DisablePyPIMirror,
    [switch]$DisableHuggingFaceMirror,
    [string]$UseCustomHuggingFaceMirror,
    [switch]$DisableGithubMirror,
    [string]$UseCustomGithubMirror,
    [switch]$DisableUV,
    [switch]$DisableCUDAMalloc,
    [switch]$DisableModelMirror,
    [switch]$NoPause
)
# Qwen TTS WebUI Installer 版本和检查更新间隔
$script:QWEN_TTS_WEBUI_INSTALLER_VERSION = 213
$script:UPDATE_TIME_SPAN = 3600
# SD WebUI All In One 内核最低版本
$script:CORE_MINIMUM_VER = "2.1.17"


# 初始化环境变量
function Initialize-EnvPath {
    Write-Log "初始化环境变量"
    $python_path = Join-NormalizedPath $PSScriptRoot "python"
    $python_extra_path = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "python"
    $python_scripts_path = Join-NormalizedPath $PSScriptRoot "python" "Scripts"
    $python_scripts_extra_path = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "python" "Scripts"
    $python_bin_path = Join-NormalizedPath $PSScriptRoot "python" "bin"
    $python_bin_extra_path = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "python" "bin"
    $git_path = Join-NormalizedPath $PSScriptRoot "git" "bin"
    $git_extra_path = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "git" "bin"
    $sep = $([System.IO.Path]::PathSeparator)
    $env:PATH = "${python_bin_extra_path}${sep}${python_extra_path}${sep}${python_scripts_extra_path}${sep}${git_extra_path}${sep}${python_bin_path}${sep}${python_path}${sep}${python_scripts_path}${sep}${git_path}${sep}${env:PATH}"

    $env:UV_CONFIG_FILE = Join-NormalizedPath $PSScriptRoot "cache" "uv.toml"
    $env:PIP_CONFIG_FILE = Join-NormalizedPath $PSScriptRoot "cache" "pip.ini"
    $env:PIP_DISABLE_PIP_VERSION_CHECK = 1
    $env:PIP_NO_WARN_SCRIPT_LOCATION = 0
    $env:UV_LINK_MODE = "copy"
    $env:PYTHONUTF8 = 1
    $env:PYTHONIOENCODING = "utf-8"
    $env:PYTHONUNBUFFERED = 1
    $env:PYTHONNOUSERSITE = 1
    $env:PYTHONFAULTHANDLER = 1
    $env:CACHE_HOME = Join-NormalizedPath $PSScriptRoot "cache"
    $env:QWEN_TTS_WEBUI_PATH = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX
    $env:QWEN_TTS_WEBUI_ROOT = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX
    $env:SD_WEBUI_ALL_IN_ONE_LAUNCH_PATH = $PSScriptRoot
    $env:SD_WEBUI_ALL_IN_ONE_LOGGER_NAME = "Qwen TTS WebUI Installer"
    $env:SD_WEBUI_ALL_IN_ONE_LOGGER_LEVEL = 20
    $env:SD_WEBUI_ALL_IN_ONE_LOGGER_COLOR = 1
    $env:SD_WEBUI_ALL_IN_ONE_RETRY_TIMES = 3
    $env:SD_WEBUI_ALL_IN_ONE_PATCHER = 0
    $env:SD_WEBUI_ALL_IN_ONE_EXTRA_PYPI_MIRROR = 0
    $env:SD_WEBUI_ALL_IN_ONE_SET_CACHE_PATH = 1
    $env:SD_WEBUI_ALL_IN_ONE_SET_CONFIG = 1
    $env:SD_WEBUI_ALL_IN_ONE_RAISE_WEBUI_RUNTIME_ERROR = 0
    $env:SD_WEBUI_ALL_IN_ONE_RAISE_CHECK_ENV_ERROR_ON_LAUNCH = 0

    New-Item -ItemType Directory -Path $env:CACHE_HOME -Force > $null
    Write-FileWithStreamWriter -Path (Join-NormalizedPath $env:CACHE_HOME "uv.toml") -Value "" -Encoding UTF8
    Write-FileWithStreamWriter -Path (Join-NormalizedPath $env:CACHE_HOME "pip.ini") -Value "" -Encoding UTF8
}


# 日志输出
function Write-Log {
    [CmdletBinding()]
    param(
        [string]$Message,
        [ValidateSet("DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL")]
        [string]$Level = "INFO",
        [string]$Name = "Qwen TTS WebUI Installer"
    )
    Write-Host "[" -NoNewline
    Write-Host $Name -ForegroundColor Blue -NoNewline
    Write-Host "]-|" -NoNewline
    Write-Host (Get-Date -Format "HH:mm:ss") -ForegroundColor Gray -NoNewline
    Write-Host "|-" -NoNewline
    switch ($Level) {
        "DEBUG"    { Write-Host "DEBUG" -ForegroundColor Cyan -NoNewline }
        "INFO"     { Write-Host "INFO" -ForegroundColor Green -NoNewline }
        "WARNING"  { Write-Host "WARNING" -ForegroundColor Yellow -NoNewline }
        "ERROR"    { Write-Host "ERROR" -ForegroundColor Red -NoNewline }
        "CRITICAL" { Write-Host "CRITICAL" -ForegroundColor White -BackgroundColor Red -NoNewline }
    }
    Write-Host ": $Message"
}


# 将文本写入文件
function Write-FileWithStreamWriter {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value,
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $false)][ValidateSet("GBK", "UTF8", "UTF8BOM")][string]$Encoding = "UTF8"
    )
    process {
        try {
            $encode = $null
            switch ($Encoding.ToLower()) {
                "GBK" {
                    if ($PSVersionTable.PSVersion.Major -ge 6) {
                        [System.Text.Encoding]::RegisterProvider([System.Text.CodePagesEncodingProvider]::Instance)
                    }
                    $encode = [System.Text.Encoding]::GetEncoding("GBK")
                }
                "UTF8" {
                    $encode = New-Object System.Text.UTF8Encoding($false)
                }
                "UTF8BOM" {
                    $encode = New-Object System.Text.UTF8Encoding($true)
                }
            }
            $absolute_path = Get-NormalizedFilePath $Path
            $writer = New-Object System.IO.StreamWriter($absolute_path, $false, $encode)
            try {
                $writer.Write($Value)
            }
            finally {
                if ($null -ne $writer) {
                    $writer.Close()
                    $writer.Dispose()
                }
            }
        }
        catch {
            Write-Log "写入文件时发生错误: $($_.Exception.Message)" -Level ERROR
        }
    }
}


# 路径拼接并规范化
function Join-NormalizedPath {
    $joined = $args[0]
    for ($i = 1; $i -lt $args.Count; $i++) { $joined = Join-Path $joined $args[$i] }
    return $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($joined).TrimEnd('\', '/')
}


# 更新 SD WebUI All In One 内核
function Update-SDWebUiAllInOne {
    $content = "
import re
from importlib.metadata import version


def compare_versions(version1: str, version2: str) -> int:
    try:
        nums1 = re.sub(r'[a-zA-Z]+', '', version1).replace('-', '.').replace('+', '.').split('.')
        nums2 = re.sub(r'[a-zA-Z]+', '', version2).replace('-', '.').replace('+', '.').split('.')
    except Exception:
        return 0
    for i in range(max(len(nums1), len(nums2))):
        num1 = int(nums1[i]) if i < len(nums1) else 0
        num2 = int(nums2[i]) if i < len(nums2) else 0
        if num1 == num2:
            continue
        elif num1 > num2:
            return 1
        else:
            return -1
    return 0


def is_core_need_update(core_minimum_ver: str) -> bool:
    try:
        core_ver = version('sd-webui-all-in-one')
    except Exception:
        return True
    return compare_versions(core_ver, core_minimum_ver) < 0


if __name__ == '__main__':
    print(is_core_need_update('$script:CORE_MINIMUM_VER'))
".Trim()

    $pip_index_url = "https://pypi.python.org/simple"
    if ((!($script:DisablePyPIMirror)) -and (!(Test-Path (Join-NormalizedPath $PSScriptRoot "disable_pypi_mirror.txt")))) {
        $pip_index_url = "https://mirrors.cloud.tencent.com/pypi/simple"
    }
    Write-Log "检测 SD WebUI All In One 内核是否需要更新"
    $status = $(python -c "$content")
    if ($status -ne "True") {
        Write-Log "SD WebUI All In One 内核无需更新"
        return
    }
    Write-Log "更新 SD WebUI All In One 内核中"
    & python -m pip install -U "sd-webui-all-in-one>=$script:CORE_MINIMUM_VER" --index-url $pip_index_url
    if (!($?)) { & python -m pip install -U "sd-webui-all-in-one>=$script:CORE_MINIMUM_VER" }
    if (!($?)) {
        Write-Log "SD WebUI All In One 内核更新失败, Installer 部分功能将无法使用" -Level ERROR
        if (!($script:BuildMode)) { if (!($script:NoPause)) { Read-Host | Out-Null } }
        exit 1
    }
    Write-Log "SD WebUI All In One 内核更新成功"
}


# Qwen TTS WebUI Installer 更新检测
function Update-Installer {
    [CmdletBinding()]
    param([switch]$DisableRestart)
    $urls = @(
        "https://github.com/licyk/sd-webui-all-in-one/raw/main/installer/qwen_tts_webui_installer.ps1",
        "https://gitee.com/licyk/sd-webui-all-in-one/raw/main/installer/qwen_tts_webui_installer.ps1",
        "https://github.com/licyk/sd-webui-all-in-one/releases/download/qwen_tts_webui_installer/qwen_tts_webui_installer.ps1",
        "https://gitee.com/licyk/sd-webui-all-in-one/releases/download/qwen_tts_webui_installer/qwen_tts_webui_installer.ps1",
        "https://gitlab.com/licyk/sd-webui-all-in-one/-/raw/main/installer/qwen_tts_webui_installer.ps1"
    )
    $i = 0

    New-Item -ItemType Directory -Path $env:CACHE_HOME -Force | Out-Null

    if ((Test-Path (Join-NormalizedPath $PSScriptRoot "disable_update.txt")) -or ($script:DisableUpdate)) {
        Write-Log "检测到 disable_update.txt 更新配置文件 / -DisableUpdate 命令行参数, 已禁用 Qwen TTS WebUI Installer 的自动检查更新功能"
        return
    }

    if ($script:BuildMode) {
        Write-Log "Qwen TTS WebUI Installer 构建模式已启用, 跳过 Qwen TTS WebUI Installer 更新检查"
        return
    }

    # 获取更新时间间隔
    try {
        $last_update_time = (Get-Content (Join-NormalizedPath $PSScriptRoot "update_time.txt") -Raw).Trim() 2> $null
        $last_update_time = Get-Date $last_update_time -Format "yyyy-MM-dd HH:mm:ss"
    }
    catch {
        $last_update_time = Get-Date 0 -Format "yyyy-MM-dd HH:mm:ss"
    }
    finally {
        $update_time = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        $time_span = New-TimeSpan -Start $last_update_time -End $update_time
    }

    if ($time_span.TotalSeconds -gt $script:UPDATE_TIME_SPAN) {
        Set-Content -Encoding UTF8 -Path (Join-NormalizedPath $PSScriptRoot "update_time.txt") -Value $(Get-Date -Format "yyyy-MM-dd HH:mm:ss") # 记录更新时间
    } else {
        return
    }

    foreach ($url in $urls) {
        Write-Log "检查 Qwen TTS WebUI Installer 更新中"
        try {
            $web_request_params = @{
                Uri = $url
                UseBasicParsing = $true
                OutFile = (Join-NormalizedPath $env:CACHE_HOME "qwen_tts_webui_installer.ps1")
                TimeoutSec = 15
                ErrorAction = "Stop"
            }
            Invoke-WebRequest @web_request_params
            $latest_version = [int]$(
                Get-Content (Join-NormalizedPath $env:CACHE_HOME "qwen_tts_webui_installer.ps1") -Encoding UTF8 |
                Select-String -Pattern "QWEN_TTS_WEBUI_INSTALLER_VERSION" |
                ForEach-Object { $_.ToString() }
            )[0].Split("=")[1].Trim()
            break
        }
        catch {
            $i += 1
            if ($i -lt $urls.Length) {
                Write-Log "重试检查 Qwen TTS WebUI Installer 更新中" -Level WARNING
            } else {
                Write-Log "检查 Qwen TTS WebUI Installer 更新失败" -Level ERROR
                return
            }
        }
    }

    if ($latest_version -le $script:QWEN_TTS_WEBUI_INSTALLER_VERSION) {
        Write-Log "Qwen TTS WebUI Installer 已是最新版本"
        return
    }

    Write-Log "调用 Qwen TTS WebUI Installer 进行更新中"
    & (Join-NormalizedPath $env:CACHE_HOME "qwen_tts_webui_installer.ps1") -InstallPath $PSScriptRoot -UseUpdateMode

    if ($DisableRestart) {
        Write-Log "更新结束, 已禁用自动重新启动"
        return
    }

    $raw_params = $script:LaunchCommandLine -replace "^.*\.ps1[\s]*", ""
    Write-Log "更新结束, 重新启动 Qwen TTS WebUI Installer 管理脚本中, 使用的命令行参数: $raw_params"
    Invoke-Expression "& `"$script:OriginalScriptPath`" $raw_params"
    exit 0
}


# 更新 Aria2 (Windows) 版本
function Update-WindowsAria2 {
    $urls = @(
        "https://www.modelscope.cn/models/licyks/sd-webui-all-in-one/resolve/master/aria2/windows/amd64/aria2c.exe",
        "https://huggingface.co/licyk/sd-webui-all-in-one/resolve/main/aria2/windows/amd64/aria2c.exe"
    )
    $aria2_tmp_path = Join-NormalizedPath $env:CACHE_HOME "aria2c.exe"
    New-Item -ItemType Directory -Path $env:CACHE_HOME -Force > $null

    foreach ($url in $urls) {
        Write-Log "下载 Aria2 中"
        try {
            $web_request_params = @{
                Uri = $url
                UseBasicParsing = $true
                OutFile = $aria2_tmp_path
                TimeoutSec = 15
                ErrorAction = "Stop"
            }
            Invoke-WebRequest @web_request_params
            break
        }
        catch {
            $i += 1
            if ($i -lt $urls.Length) {
                Write-Log "重试下载 Aria2 中" -Level WARNING
            } else {
                Write-Log "Aria2 下载失败, 无法更新 Aria2, 可能会导致模型下载出现问题" -Level ERROR
                return
            }
        }
    }

    $git_cmd = Get-Command git -ErrorAction SilentlyContinue
    if ($git_cmd) {
        $git_path_prefix = Join-NormalizedPath $script:InstallPath "git"
        $git_extra_path_prefix = Join-NormalizedPath $script:InstallPath $env:CORE_PREFIX "git"
        $git_cmd = Get-NormalizedFilePath $git_cmd.Path
        if (($git_cmd) -and (($git_cmd.ToString().StartsWith($git_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)) -or ($git_cmd.ToString().StartsWith($git_extra_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)))) {
            $aria2_bin_path = Join-NormalizedPath (Split-Path -Path $git_cmd -Parent) "aria2c.exe"
        }
        else {
            $aria2_bin_path = Join-NormalizedPath $PSScriptRoot "git" "bin" "aria2c.exe"
        }
    }
    else {
        $aria2_bin_path = Join-NormalizedPath $PSScriptRoot "git" "bin" "aria2c.exe"
    }

    New-Item -ItemType Directory -Path (Split-Path -Path $aria2_bin_path -Parent) -Force | Out-Null
    Move-Item -Path $aria2_tmp_path -Destination $aria2_bin_path -Force
}


# 更新 Aria2
function Update-Aria2 {
    Write-Log "检查 Aria2 是否需要更新"
    & python -m sd_webui_all_in_one self-manager check-aria2
    if ($?) {
        Write-Log "Aria2 无需更新"
        return
    }
    Write-Log "更新 Aria2 中"
    $platform = Get-CurrentPlatform
    if ($platform -eq "windows") {
        Update-WindowsAria2
    }
    elseif ($platform -eq "linux") {
        try {
            if (Get-Command apt -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "apt" -Arguments @("update"); Invoke-SmartCommand -Command "apt" -Arguments @("install", "--only-upgrade", "aria2", "-y"); return }
            if (Get-Command yum -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "yum" -Arguments @("upgrade", "aria2", "-y"); return }
            if (Get-Command apk -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "apk" -Arguments @("add", "--upgrade", "aria2"); return }
            if (Get-Command pacman -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "pacman" -Arguments @("-Sy", "aria2", "--noconfirm"); return }
            if (Get-Command zypper -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "zypper" -Arguments @("update", "-y", "aria2"); return }
            if (Get-Command nix-env -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "nix-channel" -Arguments @("--update"); Invoke-SmartCommand -Command "nix-env" -Arguments @("-u", "aria2"); return }
        }
        catch {
            Write-Log "更新 Aria2 失败, 可能会导致模型下载出现问题" -Level ERROR
        }
    }
    elseif ($platform -eq "macos") {
        try {
            if (Get-Command brew -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "brew" -Arguments @("upgrade", "aria2"); return }
            if (Get-Command port -ErrorAction SilentlyContinue) { Invoke-SmartCommand -Command "port" -Arguments @("upgrade", "aria2"); return }
        }
        catch {
            Write-Log "更新 Aria2 失败, 可能会导致模型下载出现问题" -Level ERROR
        }
    }
}


# 获取当前平台
function Get-CurrentPlatform {
    if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        return "windows"
    }
    elseif ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Linux)) {
        return "linux"
    }
    elseif ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)) {
        return "macos"
    }
    else {
        return "unknown"
    }
}


# 获取当前架构
function Get-CurrentArchitecture {
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLower()
    } else {
        $arch = $env:PROCESSOR_ARCHITECTURE.ToLower()
    }
    switch ($arch) {
        "amd64" { "amd64" }
        "x64"   { "amd64" }
        "arm64" { "aarch64" }
        default { $arch }
    }
}

# 获取规范化路径
function Get-NormalizedFilePath {
    [CmdletBinding()]
    param ([Parameter(Mandatory = $false)][string]$Filepath)
    if (-not [string]::IsNullOrWhiteSpace($Filepath)) { return Join-NormalizedPath $Filepath }
    return $null
}


# 显示 Qwen TTS WebUI Installer 版本
function Get-Version {
    $ver = $([string]$script:QWEN_TTS_WEBUI_INSTALLER_VERSION).ToCharArray()
    $major = ($ver[0..($ver.Length - 3)])
    $minor = $ver[-2]
    $micro = $ver[-1]
    Write-Log "Qwen TTS WebUI Installer 版本: v${major}.${minor}.${micro}"
}


# 获取帮助信息
function Get-HelpMessage {
    if (!($script:Help)) { return }
    $script = Get-Command $script:OriginalScriptPath
    $common = [System.Management.Automation.Internal.CommonParameters].GetProperties().Name
    $display_params = $script.Parameters.Values | Where-Object { $_.Name -notin $common } | ForEach-Object {
        $p_name = $_.Name
        $p_type = $_.ParameterType.Name
        if ($_.ParameterType -eq [switch]) {
            $format = "-$p_name"
        }
        else {
            # 处理数组类型的显示逻辑
            # 如果是数组, PowerShell 习惯在类型名后加 []
            if ($_.ParameterType.IsArray) {
                # 移除原类型名中的 [] 或 System. 前缀, 统一格式
                $clean_type = "$($_.ParameterType.GetElementType().Name)[]"
            } else {
                $clean_type = $p_type
            }
            $format = "-$p_name <$clean_type>"
        }
        $help_msg = $_.Attributes.HelpMessage
        [PSCustomObject]@{
            Name = $format
            HelpMessage = $help_msg
        }
    }
    $usage = @"
使用:
    $((Get-Process -Id $PID).Path) ${script:OriginalScriptPath} $(foreach ($i in $display_params.Name) { "[$i]" })
"@
    $param_info = @"
参数:
$(
    foreach ($i in $display_params) {
        $text = "    $($i.Name)"
        if ($i.HelpMessage) {
            $indented_help = ($i.HelpMessage -split "`?`
" | ForEach-Object { "        $_" }) -join "`
"
            $text += "`
$indented_help"
        }
        $text + "`
`
"
    }
)
"@
    $docs_url = "更多的帮助信息请阅读 Qwen TTS WebUI Installer 使用文档: https://github.com/licyk/sd-webui-all-in-one/blob/main/docs/qwen_tts_webui_installer.md"
    Write-Host $($usage + "`
`
" + $param_info + "`
" + $docs_url) -ForegroundColor White
    exit 0
}


# 设置内核路径前缀
function Set-CorePrefix {
    $target_prefix = $null
    $prefix_list = @("core", "qwen-tts-webui*")
    if ($script:CorePrefix -or (Test-Path (Join-NormalizedPath $PSScriptRoot "core_prefix.txt"))) {
        Write-Log "检测到 core_prefix.txt 配置文件 / -CorePrefix 命令行参数, 使用自定义内核路径前缀"
        $origin_core_prefix = if ($script:CorePrefix) {
            $script:CorePrefix
        } else {
            (Get-Content (Join-NormalizedPath $PSScriptRoot "core_prefix.txt") -Raw -Encoding UTF8).Trim()
        }
        $origin_core_prefix = $origin_core_prefix.TrimEnd('\', '/')
        if ([System.IO.Path]::IsPathRooted($origin_core_prefix)) {
            $from_uri = New-Object System.Uri($PSScriptRoot.Replace('\', '/') + '/')
            $to_uri = New-Object System.Uri($origin_core_prefix.Replace('\', '/'))
            $target_prefix = $from_uri.MakeRelativeUri($to_uri).ToString().Trim('/')
            Write-Log "转换绝对路径为内核路径前缀: $origin_core_prefix -> $target_prefix"
        } else {
            $target_prefix = $origin_core_prefix
        }
    }
    else {
        foreach ($i in $prefix_list) {
            $found_dir = Get-ChildItem -Path $PSScriptRoot -Directory -Filter $i -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($found_dir) {
                $target_prefix = $found_dir.Name
                break
            }
        }
    }
    if ([string]::IsNullOrWhiteSpace($target_prefix)) {
        $target_prefix = "core"
    }
    $env:CORE_PREFIX = $target_prefix
    $full_core_path = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX
    Write-Log "当前内核路径前缀: $env:CORE_PREFIX"
    Write-Log "完整内核路径: $full_core_path"
}


# 代理配置
function Set-Proxy {
    [CmdletBinding()]
    param ([Parameter()][switch]$Legacy)
    $env:NO_PROXY = "localhost,127.0.0.1,::1"
    if ($script:DisableProxy -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_proxy.txt"))) {
        Write-Log "检测到本地存在 disable_proxy.txt 代理配置文件 / -DisableProxy 命令行参数, 禁用自动设置代理"
        return
    }
    if ($script:UseCustomProxy -or (Test-Path (Join-NormalizedPath $PSScriptRoot "proxy.txt"))) {
        if ($script:UseCustomProxy) {
            $proxy_value = $script:UseCustomProxy
        } else {
            $proxy_value = (Get-Content (Join-NormalizedPath $PSScriptRoot "proxy.txt") -Raw -Encoding UTF8).Trim()
        }
        $env:HTTP_PROXY = $proxy_value
        $env:HTTPS_PROXY = $proxy_value
        Write-Log "检测到本地存在 proxy.txt 代理配置文件 / -UseCustomProxy 命令行参数, 已读取代理配置文件并设置代理"
        return
    }
    if ($Legacy) {
        $proxy_value = & python -m sd_webui_all_in_one self-manager get-proxy
        if (![string]::IsNullOrWhiteSpace($proxy_value)) {
            $env:HTTP_PROXY = $proxy_value
            $env:HTTPS_PROXY = $proxy_value
            Write-Log "检测到系统设置了代理, 已读取系统中的代理配置并设置代理"
        }
    } else {
        $env:SD_WEBUI_ALL_IN_ONE_PROXY = 1
        Write-Log "使用自动检测代理模式进行代理配置"
    }
}


# 配置 PyPI 镜像源
function Set-PyPIMirror {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    if ($script:DisablePyPIMirror -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_pypi_mirror.txt"))) {
        Write-Log "检测到 disable_pypi_mirror.txt 配置文件 / -DisablePyPIMirror 命令行参数, 已将 PyPI 源切换至官方源"
        $ArrayList.Add("--no-pypi-mirror") | Out-Null
        return
    }
    Write-Log "使用 PyPI 镜像源"
}


# 设置模型下载源
function Set-ModelMirror {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    $ArrayList.Add("--source") | Out-Null
    if ((!(Test-Path (Join-NormalizedPath $PSScriptRoot "disable_model_mirror.txt"))) -and (!($script:DisableModelMirror))) {
        Write-Log "使用 ModelScope 模型下载源"
        $ArrayList.Add("modelscope") | Out-Null
    } else {
        Write-Log "检测到 disable_model_mirror.txt 配置文件 / -DisableModelMirror 命令行参数, 已将模型下载源切换至 HuggingFace 源"
        $ArrayList.Add("huggingface") | Out-Null
    }
}


# HuggingFace 镜像源
function Set-HuggingFaceMirror {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    if ($script:DisableHuggingFaceMirror -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_hf_mirror.txt"))) {
        Write-Log "检测到本地存在 disable_hf_mirror.txt 镜像源配置文件 / -DisableHuggingFaceMirror 命令行参数, 禁用自动设置 HuggingFace 镜像源"
        $ArrayList.Add("--no-hf-mirror") | Out-Null
        return
    }
    if ($script:UseCustomHuggingFaceMirror -or (Test-Path (Join-NormalizedPath $PSScriptRoot "hf_mirror.txt"))) {
        if ($script:UseCustomHuggingFaceMirror) {
            $hf_mirror_value = $script:UseCustomHuggingFaceMirror
        } else {
            $hf_mirror_value = (Get-Content (Join-NormalizedPath $PSScriptRoot "hf_mirror.txt") -Raw -Encoding UTF8).Trim()
        }
        $ArrayList.Add("--custom-hf-mirror") | Out-Null
        $ArrayList.Add($hf_mirror_value) | Out-Null
        Write-Log "检测到本地存在 hf_mirror.txt 配置文件 / -UseCustomHuggingFaceMirror 命令行参数, 已读取该配置并设置 HuggingFace 镜像源"
        return
    }
    Write-Log "使用默认 HuggingFace 镜像源"
}


# 设置 Github 镜像源
function Set-GithubMirror {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    if (Test-Path (Join-NormalizedPath $PSScriptRoot ".gitconfig")) {
        Remove-Item -Path (Join-NormalizedPath $PSScriptRoot ".gitconfig") -Force -Recurse
    }
    if ($script:DisableGithubMirror -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_gh_mirror.txt"))) {
        Write-Log "检测到本地存在 disable_gh_mirror.txt Github 镜像源配置文件 / -DisableGithubMirror 命令行参数, 禁用 Github 镜像源"
        $ArrayList.Add("--no-github-mirror") | Out-Null
        return
    }
    if ($script:UseCustomGithubMirror -or (Test-Path (Join-NormalizedPath $PSScriptRoot "gh_mirror.txt"))) {
        if ($script:UseCustomGithubMirror) {
            $github_mirror = $script:UseCustomGithubMirror
        } else {
            $github_mirror = (Get-Content (Join-NormalizedPath $PSScriptRoot "gh_mirror.txt") -Raw -Encoding UTF8).Trim()
        }
        Write-Log "检测到本地存在 gh_mirror.txt Github 镜像源配置文件 / -UseCustomGithubMirror 命令行参数, 已读取 Github 镜像源配置文件并设置 Github 镜像源"
        $ArrayList.Add("--custom-github-mirror") | Out-Null
        $ArrayList.Add($github_mirror) | Out-Null
    }
}


# 设置 uv 的使用状态
function Set-uv {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    if ($script:DisableUV -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_uv.txt"))) {
        Write-Log "检测到 disable_uv.txt 配置文件 / -DisableUV 命令行参数, 已禁用 uv, 使用 Pip 作为 Python 包管理器"
        $ArrayList.Add("--no-uv") | Out-Null
    } else {
        Write-Log "默认启用 uv 作为 Python 包管理器, 加快 Python 软件包的安装速度"
        Write-Log "当 uv 安装 Python 软件包失败时, 将自动切换成 Pip 重试 Python 软件包的安装"
    }
}


# 设置 CUDA 内存分配器
function Set-PyTorchCUDAMemoryAlloc {
    [CmdletBinding()]
    param ([System.Collections.ArrayList]$ArrayList)
    if ($script:DisableCUDAMalloc -or (Test-Path (Join-NormalizedPath $PSScriptRoot "disable_set_pytorch_cuda_memory_alloc.txt"))) {
        Write-Log "检测到 disable_set_pytorch_cuda_memory_alloc.txt 配置文件 / -DisableCUDAMalloc 命令行参数, 已禁用自动设置 CUDA 内存分配器"
        $ArrayList.Add("--no-cuda-malloc") | Out-Null
    }
}


# 创建 Windows 快捷方式
function Add-WindowsShortcut {
    [CmdletBinding()]
    param (
        [string]$Name,
        [string]$IconPath
    )
    $shell = New-Object -ComObject WScript.Shell
    $desktop = $([System.Environment]::GetFolderPath("Desktop"))
    $shortcut_path = Join-NormalizedPath $desktop "${Name}.lnk"
    $shortcut = $shell.CreateShortcut($shortcut_path)
    $shortcut.TargetPath = (Get-Process -Id $PID).Path
    $launch_script_path = Join-NormalizedPath $PSScriptRoot "launch.ps1"
    $shortcut.Arguments = "-ExecutionPolicy Bypass -File `"$launch_script_path`""
    $shortcut.IconLocation = $IconPath
    $shortcut.Save()
    Copy-Item -Path $shortcut_path -Destination (Join-NormalizedPath $([System.Environment]::GetFolderPath("ApplicationData")) "Microsoft" "Windows" "Start Menu" "Programs") -Force
}


# 创建 Linux 快捷方式
function Add-LinuxShortcut {
    [CmdletBinding()]
    param (
        [string]$Name,
        [string]$IconPath
    )
    $pwsh_bin = (Get-Process -Id $PID).Path
    $launch_script_path = Join-NormalizedPath $PSScriptRoot "launch.ps1"
    $desktop = $([System.Environment]::GetFolderPath("Desktop"))
    $shortcut_path = Join-NormalizedPath $desktop "${Name}.desktop"
    $content = "
[Desktop Entry]
Encoding=UTF-8
Version=1.0
Name=$Name
Comment=Installer 启动脚本
Icon=$IconPath
Exec=`"$pwsh_bin`" -ExecutionPolicy Bypass -File `"$launch_script_path`" %f
Terminal=true
startupNotify=true
Type=Application
".Trim()
    Write-FileWithStreamWriter -Path $shortcut_path -Encoding UTF8 -Value $content
    $local_app_path = Join-NormalizedPath $([System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)) ".local" "share" "applications"
    $local_app_shortcut_path = Join-NormalizedPath $local_app_path "${Name}.desktop"
    New-Item -ItemType Directory -Path $local_app_path -Force | Out-Null
    Copy-Item -Path $shortcut_path -Destination $local_app_shortcut_path -Force
    & chmod +x "$shortcut_path"
    & chmod +x "$local_app_shortcut_path"
}


# 创建 MacOS 快捷方式
function Add-MacOSShortcut {
    [CmdletBinding()]
    param (
        [string]$Name,
        [string]$IconPath
    )

    $pwsh_bin = (Get-Process -Id $PID).Path
    $launch_script_path = Join-NormalizedPath $PSScriptRoot "launch.ps1"
    $desktop = $([System.Environment]::GetFolderPath("Desktop"))

    $app_path = Join-NormalizedPath $desktop "${Name}.app"
    $contents_path = Join-NormalizedPath $app_path "Contents"
    $macos_path = Join-NormalizedPath $contents_path "MacOS"
    $resources_path = Join-NormalizedPath $contents_path "Resources"

    New-Item -ItemType Directory -Path $macos_path -Force | Out-Null
    New-Item -ItemType Directory -Path $resources_path -Force | Out-Null

    $working_dir_for_shell = $PSScriptRoot.Replace('"', '\"').Replace('$', '\$')
    $pwsh_bin_for_shell = $pwsh_bin.Replace('"', '\"').Replace('$', '\$')
    $launch_script_for_shell = $launch_script_path.Replace('"', '\"').Replace('$', '\$')

    $executable_path = Join-NormalizedPath $macos_path "launcher"
    $sh_content = @"
#!/bin/bash

osascript <<'APPLESCRIPT'
tell application "Terminal"
    activate
    do script "cd \"$working_dir_for_shell\" || exit 1; exec \"$pwsh_bin_for_shell\" -NoExit -ExecutionPolicy Bypass -File \"$launch_script_for_shell\""
end tell
APPLESCRIPT
"@
    $sh_content = $sh_content.Replace("``
", "`
").Replace("`", "`
")
    Write-FileWithStreamWriter -Path $executable_path -Encoding UTF8 -Value $sh_content
    & chmod +x "$executable_path"

    $plist_path = Join-NormalizedPath $contents_path "Info.plist"
    $plist_content = @"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>launcher</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon.icns</string>
    <key>CFBundleName</key>
    <string>${Name}</string>
    <key>CFBundleDisplayName</key>
    <string>${Name}</string>
    <key>CFBundleIdentifier</key>
    <string>local.sdwebuiallinone.$($Name.ToLower() -replace '[^a-z0-9]', '')</string>
    <key>CFBundleVersion</key>
    <string>1.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleSignature</key>
    <string>????</string>
    <key>LSBackgroundOnly</key>
    <false/>
    <key>LSMinimumSystemVersion</key>
    <string>10.13</string>
</dict>
</plist>
"@
    Write-FileWithStreamWriter -Path $plist_path -Encoding UTF8 -Value $plist_content

    if (Test-Path $IconPath) {
        Copy-Item -Path $IconPath -Destination (Join-NormalizedPath $resources_path "AppIcon.icns") -Force
    }

    $applications_folder = "/Applications"
    if (Test-Path $applications_folder) {
        Copy-Item -Path $app_path -Destination $applications_folder -Recurse -Force
    }
}


# 下载应用图标
function Get-AppIcon {
    [CmdletBinding()]
    param ([Parameter(Mandatory=$true)][Hashtable]$IconMap)
    $platform = Get-CurrentPlatform
    if (-not $IconMap.ContainsKey($platform)) {
        Write-Log "未找到平台 [$platform] 的图标配置" -Level WARNING
        return $null
    }
    $config = $IconMap[$platform]
    $fileName = $config.FileName
    $localIconPath = Join-NormalizedPath $PSScriptRoot $fileName
    if (Test-Path $localIconPath) { return $localIconPath }
    foreach ($url in $config.Urls) {
        try {
            Write-Log "正在下载 $platform 图标: $url"
            $web_request_params = @{
                Uri = $url
                UseBasicParsing = $true
                OutFile = $localIconPath
                TimeoutSec = 15
                ErrorAction = "Stop"
            }
            Invoke-WebRequest @web_request_params
            if (Test-Path $localIconPath) { return $localIconPath }
        }
        catch {
            Write-Log "链接失效: $url" -Level WARNING
        }
    }
    return $null
}


# 创建快捷方式
function New-AppShortcut {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory=$true)][string]$Name,
        [Parameter(Mandatory=$true)][Hashtable]$IconMap
    )
    $finalIconPath = Get-AppIcon -IconMap $IconMap
    if (-not $finalIconPath) {
        Write-Log "图标获取失败，跳过创建快捷方式" -Level ERROR
        return
    }
    $platform = Get-CurrentPlatform
    switch ($platform) {
        "windows" { Add-WindowsShortcut -Name $Name -IconPath $finalIconPath }
        "linux"   { Add-LinuxShortcut -Name $Name -IconPath $finalIconPath }
        "macos"   { Add-MacOSShortcut -Name $Name -IconPath $finalIconPath }
    }
}


# 测试 Python 和 Git 可用性
function Test-PythonAndGit {
    # Python
    $python_cmd = Get-Command python -ErrorAction SilentlyContinue
    if ($python_cmd) {
        $python_cmd = Get-NormalizedFilePath $python_cmd.Path
        $python_path_prefix = Join-NormalizedPath $PSScriptRoot "python"
        $python_extra_path_prefix = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "python"
        if (-not (($python_cmd) -and (($python_cmd.ToString().StartsWith($python_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)) -or ($python_cmd.ToString().StartsWith($python_extra_path_prefix, [System.StringComparison]::OrdinalIgnoreCase))))) {
            Write-Log "检测到当前使用的 Python 路径为 ${python_cmd}, 但未在 ${python_path_prefix} 或 ${python_extra_path_prefix} 这两个受 Qwen TTS WebUI Installer 管理的 Python 路径, 即当前正在使用外部的 Python 环境, 这可能会导致一些运行环境问题, 可尝试运行 launch_qwen_tts_webui_installer.ps1 修复运行环境" -Level ERROR
        }
    } else {
        Write-Log "检测到当前环境中未安装任何 Python, 这将导致运行时发生异常, 请运行 launch_qwen_tts_webui_installer.ps1 修复运行环境" -Level ERROR
    }

    # Git
    $git_cmd = Get-Command git -ErrorAction SilentlyContinue
    if ($git_cmd) {
        if ((Get-CurrentPlatform) -eq "windows") {
            $git_cmd = Get-NormalizedFilePath $git_cmd.Path
            $git_path_prefix = Join-NormalizedPath $PSScriptRoot "git"
            $git_extra_path_prefix = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "git"
            if (-not (($git_cmd) -and (($git_cmd.ToString().StartsWith($git_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)) -or ($git_cmd.ToString().StartsWith($git_extra_path_prefix, [System.StringComparison]::OrdinalIgnoreCase))))) {
                Write-Log "检测到当前使用的 Git 路径为 ${git_cmd}, 但未在 ${git_path_prefix} 或 ${git_extra_path_prefix} 这两个受 Qwen TTS WebUI Installer 管理的 Git 路径, 即当前正在使用外部的 Git 环境, 这可能会导致一些运行环境问题, 可尝试运行 launch_qwen_tts_webui_installer.ps1 修复运行环境" -Level ERROR
            }
        }
    } else {
        Write-Log "检测到当前环境中未安装任何 Git, 这将导致运行时发生异常, 请运行 launch_qwen_tts_webui_installer.ps1 修复运行环境" -Level ERROR
    }
}


Export-ModuleMember -Function `
    Initialize-EnvPath, `
    Write-Log, `
    Write-FileWithStreamWriter, `
    Update-SDWebUiAllInOne, `
    Update-Installer, `
    Update-Aria2, `
    Get-Version, `
    Get-HelpMessage, `
    Set-CorePrefix, `
    Set-Proxy, `
    Set-ModelMirror, `
    Set-PyPIMirror, `
    Set-HuggingFaceMirror, `
    Set-GithubMirror, `
    Set-uv, `
    Set-PyTorchCUDAMemoryAlloc, `
    Join-NormalizedPath, `
    Get-NormalizedFilePath, `
    Get-CurrentPlatform, `
    Get-CurrentArchitecture, `
    New-AppShortcut, `
    Test-PythonAndGit