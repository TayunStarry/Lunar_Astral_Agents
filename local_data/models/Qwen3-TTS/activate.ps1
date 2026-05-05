param (
    [Parameter(HelpMessage=@"
获取 Qwen TTS WebUI Installer 的帮助信息
"@)][switch]$Help,

    [Parameter(HelpMessage=@"
设置内核的路径前缀, 默认路径前缀为 core
"@)][string]$CorePrefix,

    [Parameter(HelpMessage=@"
禁用 PyPI 镜像源, 使用 PyPI 官方源下载 Python 软件包
"@)][switch]$DisablePyPIMirror,

    [Parameter(HelpMessage=@"
禁用 Qwen TTS WebUI Installer 自动设置 Github 镜像源
"@)][switch]$DisableGithubMirror,

    [Parameter(HelpMessage=@"
使用自定义的 Github 镜像站地址
"@)][string]$UseCustomGithubMirror,

    [Parameter(HelpMessage=@"
禁用 Qwen TTS WebUI Installer 自动设置代理服务器
"@)][switch]$DisableProxy,

    [Parameter(HelpMessage=@"
使用自定义的代理服务器地址, 例如代理服务器地址为 http://127.0.0.1:10809, 则使用 -UseCustomProxy `"http://127.0.0.1:10809`" 设置代理服务器地址
"@)][string]$UseCustomProxy,

    [Parameter(HelpMessage=@"
禁用 HuggingFace 镜像源, 不使用 HuggingFace 镜像源下载文件
"@)][switch]$DisableHuggingFaceMirror,

    [Parameter(HelpMessage=@"
使用自定义 HuggingFace 镜像源地址, 例如代理服务器地址为 https://hf-mirror.com, 则使用 -UseCustomHuggingFaceMirror `"https://hf-mirror.com`" 设置 HuggingFace 镜像源地址
"@)][string]$UseCustomHuggingFaceMirror,

    [Parameter(HelpMessage=@"
脚本执行完成后不暂停, 直接退出
"@)][switch]$NoPause
)
try {
    $config = @{
        OriginalScriptPath = $script:PSCommandPath
        LaunchCommandLine = $script:MyInvocation.Line
        Help = $script:Help
        CorePrefix = $script:CorePrefix
        DisablePyPIMirror = $script:DisablePyPIMirror
        DisableGithubMirror = $script:DisableGithubMirror
        UseCustomGithubMirror = $script:UseCustomGithubMirror
        DisableProxy = $script:DisableProxy
        UseCustomProxy = $script:UseCustomProxy
        DisableHuggingFaceMirror = $script:DisableHuggingFaceMirror
        UseCustomHuggingFaceMirror = $script:UseCustomHuggingFaceMirror
        NoPause = $script:NoPause
    }
    (Import-Module (Join-Path $PSScriptRoot "modules.psm1") -Function "Join-NormalizedPath", "Initialize-EnvPath", "Write-Log", "Set-CorePrefix", "Get-Version", "Set-Proxy", "Get-NormalizedFilePath", "Get-HelpMessage", "Test-PythonAndGit" -PassThru -Force -ErrorAction Stop).Invoke({
        param ($cfg)
        $script:OriginalScriptPath = $cfg.OriginalScriptPath
        $script:LaunchCommandLine = $cfg.LaunchCommandLine
        $script:Help = $cfg.Help
        $script:CorePrefix = $cfg.CorePrefix
        $script:DisablePyPIMirror = $cfg.DisablePyPIMirror
        $script:DisableGithubMirror = $cfg.DisableGithubMirror
        $script:UseCustomGithubMirror = $cfg.UseCustomGithubMirror
        $script:DisableProxy = $cfg.DisableProxy
        $script:UseCustomProxy = $cfg.UseCustomProxy
        $script:DisableHuggingFaceMirror = $cfg.DisableHuggingFaceMirror
        $script:UseCustomHuggingFaceMirror = $cfg.UseCustomHuggingFaceMirror
        $script:NoPause = $cfg.NoPause
    }, $config)
}
catch {
    Write-Error "导入 Installer 模块发生错误: $_"
    Write-Host "这可能是 Installer 文件出现了损坏, 请运行 " -ForegroundColor White -NoNewline
    Write-Host "launch_qwen_tts_webui_installer.ps1" -ForegroundColor Yellow -NoNewline
    Write-Host " 脚本修复该问题" -ForegroundColor White
    if (!($script:NoPause)) { Read-Host | Out-Null }
    exit 1
}


# PyPI 镜像源
$PIP_INDEX_ADDR = "https://mirrors.cloud.tencent.com/pypi/simple"
$PIP_INDEX_ADDR_ORI = "https://pypi.python.org/simple"
$PIP_EXTRA_INDEX_ADDR = "https://mirrors.cernet.edu.cn/pypi/web/simple"
$PIP_EXTRA_INDEX_ADDR_ORI = "https://download.pytorch.org/whl"
$PIP_FIND_ADDR = "https://mirrors.aliyun.com/pytorch-wheels/torch_stable.html"
$PIP_FIND_ADDR_ORI = "https://download.pytorch.org/whl/torch_stable.html"
$USE_PIP_MIRROR = if ((!(Test-Path (Join-NormalizedPath $PSScriptRoot "disable_pypi_mirror.txt"))) -and (!($script:DisablePyPIMirror))) { $true } else { $false }
$PIP_INDEX_MIRROR = if ($USE_PIP_MIRROR) { $PIP_INDEX_ADDR } else { $PIP_INDEX_ADDR_ORI }
$PIP_EXTRA_INDEX_MIRROR = if ($USE_PIP_MIRROR) { $PIP_EXTRA_INDEX_ADDR } else { $PIP_EXTRA_INDEX_ADDR_ORI }
$PIP_FIND_MIRROR = if ($USE_PIP_MIRROR) { $PIP_FIND_ADDR } else { $PIP_FIND_ADDR_ORI }
# 环境变量
$env:PIP_INDEX_URL = "$PIP_INDEX_MIRROR"
$env:PIP_EXTRA_INDEX_URL = if ($PIP_EXTRA_INDEX_MIRROR -ne $PIP_EXTRA_INDEX_MIRROR_PYTORCH) { "$PIP_EXTRA_INDEX_MIRROR $PIP_EXTRA_INDEX_MIRROR_PYTORCH".Trim() } else { $PIP_EXTRA_INDEX_MIRROR }
$env:PIP_FIND_LINKS = "$PIP_FIND_MIRROR"
$env:UV_DEFAULT_INDEX = "$PIP_INDEX_MIRROR"
$env:UV_INDEX = if ($PIP_EXTRA_INDEX_MIRROR -ne $PIP_EXTRA_INDEX_MIRROR_PYTORCH) { "$PIP_EXTRA_INDEX_MIRROR $PIP_EXTRA_INDEX_MIRROR_PYTORCH".Trim() } else { $PIP_EXTRA_INDEX_MIRROR }
$env:UV_FIND_LINKS = "$PIP_FIND_MIRROR"
$env:UV_LINK_MODE = "copy"
$env:UV_HTTP_TIMEOUT = 30
$env:UV_CONCURRENT_DOWNLOADS = 50
$env:UV_INDEX_STRATEGY = "unsafe-best-match"
$env:PIP_DISABLE_PIP_VERSION_CHECK = 1
$env:PIP_NO_WARN_SCRIPT_LOCATION = 0
$env:PIP_TIMEOUT = 30
$env:PIP_RETRIES = 5
$env:PIP_PREFER_BINARY = 1
$env:PIP_YES = 1
$env:PYTHONUTF8 = 1
$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUNBUFFERED = 1
$env:PYTHONNOUSERSITE = 1
$env:PYTHONFAULTHANDLER = 1
$env:PYTHONWARNINGS = "ignore:::torchvision.transforms.functional_tensor,ignore::UserWarning,ignore::FutureWarning,ignore::DeprecationWarning"
$env:GRADIO_ANALYTICS_ENABLED = "False"
$env:HF_HUB_DISABLE_SYMLINKS_WARNING = 1
$env:BITSANDBYTES_NOWELCOME = 1
$env:ClDeviceGlobalMemSizeAvailablePercent = 100
$env:CUDA_MODULE_LOADING = "LAZY"
$env:TORCH_CUDNN_V8_API_ENABLED = 1
$env:USE_LIBUV = 0
$env:SYCL_CACHE_PERSISTENT = 1
$env:TF_CPP_MIN_LOG_LEVEL = 3
$env:SAFETENSORS_FAST_GPU = 1
$env:CACHE_HOME = Join-NormalizedPath $PSScriptRoot "cache"
$env:HF_HOME = Join-NormalizedPath $PSScriptRoot "cache" "huggingface"
$env:MATPLOTLIBRC = Join-NormalizedPath $PSScriptRoot "cache"
$env:MODELSCOPE_CACHE = Join-NormalizedPath $PSScriptRoot "cache" "modelscope" "hub"
$env:MS_CACHE_HOME = Join-NormalizedPath $PSScriptRoot "cache" "modelscope" "hub"
$env:SYCL_CACHE_DIR = Join-NormalizedPath $PSScriptRoot "cache" "libsycl_cache"
$env:TORCH_HOME = Join-NormalizedPath $PSScriptRoot "cache" "torch"
$env:U2NET_HOME = Join-NormalizedPath $PSScriptRoot "cache" "u2net"
$env:XDG_CACHE_HOME = Join-NormalizedPath $PSScriptRoot "cache"
$env:PIP_CACHE_DIR = Join-NormalizedPath $PSScriptRoot "cache" "pip"
$env:PYTHONPYCACHEPREFIX = Join-NormalizedPath $PSScriptRoot "cache" "pycache"
$env:TORCHINDUCTOR_CACHE_DIR = Join-NormalizedPath $PSScriptRoot "cache" "torchinductor"
$env:TRITON_CACHE_DIR = Join-NormalizedPath $PSScriptRoot "cache" "triton"
$env:UV_CACHE_DIR = Join-NormalizedPath $PSScriptRoot "cache" "uv"
$env:QWEN_TTS_WEBUI_PATH = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX
$env:QWEN_TTS_WEBUI_INSTALLER_ROOT = $PSScriptRoot


# 提示符信息
function global:prompt {
    "$(Write-Host "[Qwen TTS WebUI Env]" -ForegroundColor Green -NoNewLine) $(Get-Location)> "
}

function global:pip {
    python -m pip @args
}

function global:sd-webui-all-in-one {
    & python -m sd_webui_all_in_one @args
}

Set-Alias pip3 pip
Set-Alias python3 python


# 列出 Qwen TTS WebUI Installer 内置命令
function global:List-CMD {
    Write-Host "
==================================
Qwen TTS WebUI Installer created by licyk
哔哩哔哩：https://space.bilibili.com/46497516
Github：https://github.com/licyk
==================================

当前可用的 Qwen TTS WebUI Installer 内置命令：

    List-CMD

更多帮助信息可在 Qwen TTS WebUI Installer 文档中查看: https://github.com/licyk/sd-webui-all-in-one/blob/main/docs/qwen_tts_webui_installer.md
"
}


# PyPI 镜像源状态
function Get-PyPIMirrorStatus {
    if ($USE_PIP_MIRROR) {
        Write-Log "使用 PyPI 镜像源"
    } else {
        Write-Log "检测到 disable_pypi_mirror.txt 配置文件 / -DisablePyPIMirror, 命令行参数 已将 PyPI 源切换至官方源"
    }
}


# HuggingFace 镜像源
function Set-HuggingFaceMirror {
    if ((Test-Path (Join-NormalizedPath $PSScriptRoot "disable_hf_mirror.txt")) -or ($script:DisableHuggingFaceMirror)) { # 检测是否禁用了自动设置 HuggingFace 镜像源
        Write-Log "检测到本地存在 disable_hf_mirror.txt 镜像源配置文件 / -DisableHuggingFaceMirror 命令行参数, 禁用自动设置 HuggingFace 镜像源"
        return
    }

    if ((Test-Path (Join-NormalizedPath $PSScriptRoot "hf_mirror.txt")) -or ($script:UseCustomHuggingFaceMirror)) { # 本地存在 HuggingFace 镜像源配置
        if ($script:UseCustomHuggingFaceMirror) {
            $hf_mirror_value = $script:UseCustomHuggingFaceMirror
        } else {
            $hf_mirror_value = (Get-Content (Join-NormalizedPath $PSScriptRoot "hf_mirror.txt") -Raw).Trim()
        }
        $env:HF_ENDPOINT = $hf_mirror_value
        Write-Log "检测到本地存在 hf_mirror.txt 配置文件 / -UseCustomHuggingFaceMirror 命令行参数, 已读取该配置并设置 HuggingFace 镜像源"
    } else { # 使用默认设置
        $env:HF_ENDPOINT = "https://hf-mirror.com"
        Write-Log "使用默认 HuggingFace 镜像源"
    }
}


# Github 镜像源
function Set-GithubMirrorLegecy {
    $env:GIT_CONFIG_GLOBAL = Join-NormalizedPath $PSScriptRoot ".gitconfig" # 设置 Git 配置文件路径
    if (Test-Path (Join-NormalizedPath $PSScriptRoot ".gitconfig")) {
        Remove-Item -Path (Join-NormalizedPath $PSScriptRoot ".gitconfig") -Force -Recurse
    }

    # 默认 Git 配置
    git config --global --add safe.directory '*'
    git config --global core.longpaths true

    if ((Test-Path (Join-NormalizedPath $PSScriptRoot "disable_gh_mirror.txt")) -or ($script:DisableGithubMirror)) { # 禁用 Github 镜像源
        Write-Log "检测到本地存在 disable_gh_mirror.txt Github 镜像源配置文件 / -DisableGithubMirror 命令行参数, 禁用 Github 镜像源"
        return
    }

    # 使用自定义 Github 镜像源
    if ((Test-Path (Join-NormalizedPath $PSScriptRoot "gh_mirror.txt")) -or ($script:UseCustomGithubMirror)) {
        if ($script:UseCustomGithubMirror) {
            $github_mirror = $script:UseCustomGithubMirror
        } else {
            $github_mirror = (Get-Content (Join-NormalizedPath $PSScriptRoot "gh_mirror.txt") -Raw).Trim()
        }
        git config --global url."$github_mirror".insteadOf "https://github.com"
        Write-Log "检测到本地存在 gh_mirror.txt Github 镜像源配置文件 / -UseCustomGithubMirror 命令行参数, 已读取 Github 镜像源配置文件并设置 Github 镜像源"
    }
}


function Main {
    Get-HelpMessage
    Get-Version
    Set-CorePrefix
    Initialize-EnvPath
    Test-PythonAndGit
    Set-Proxy -Legacy
    Set-HuggingFaceMirror
    Set-GithubMirrorLegecy
    Get-PyPIMirrorStatus

    $python_cmd = Get-Command python -ErrorAction SilentlyContinue
    if ($python_cmd) {
        $python_path_prefix = Join-NormalizedPath $PSScriptRoot "python"
        $python_extra_path_prefix = Join-NormalizedPath $PSScriptRoot $env:CORE_PREFIX "python"
        $python_cmd = Get-NormalizedFilePath $python_cmd.Path
        if (($python_cmd) -and (($python_cmd.ToString().StartsWith($python_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)) -or ($python_cmd.ToString().StartsWith($python_extra_path_prefix, [System.StringComparison]::OrdinalIgnoreCase)))) {
            $env:UV_PYTHON = $python_cmd
        }
    }

    Write-Log "激活 Qwen TTS WebUI Env"
    Write-Log "更多帮助信息可在 Qwen TTS WebUI Installer 项目地址查看: https://github.com/licyk/sd-webui-all-in-one/blob/main/docs/qwen_tts_webui_installer.md"
}

###################

Main