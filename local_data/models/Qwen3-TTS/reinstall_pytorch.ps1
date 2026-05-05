param (
    [Parameter(HelpMessage=@"
获取 Qwen TTS WebUI Installer 的帮助信息
"@)][switch]$Help,

    [Parameter(HelpMessage=@"
设置内核的路径前缀, 默认路径前缀为 core
"@)][string]$CorePrefix,

    [Parameter(HelpMessage=@"
启用 Qwen TTS WebUI Installer 构建模式
"@)][switch]$BuildMode,

    [Parameter(HelpMessage=@"
(需添加 -BuildMode 启用 Qwen TTS WebUI Installer 构建模式) Qwen TTS WebUI Installer 执行完基础安装流程后调用 Qwen TTS WebUI Installer 的 reinstall_pytorch.ps1 脚本, 根据 PyTorch 版本编号安装指定的 PyTorch 版本
PyTorch 版本编号可运行 reinstall_pytorch.ps1 脚本进行查看
"@)][int]$BuildWithTorch,

    [Parameter(HelpMessage=@"
(需添加 -BuildMode 启用 Qwen TTS WebUI Installer 构建模式, 并且添加 -BuildWithTorch) 在 Qwen TTS WebUI Installer 构建模式下, 执行 reinstall_pytorch.ps1 脚本对 PyTorch 进行指定版本安装时使用强制重新安装
"@)][switch]$BuildWithTorchReinstall,

    [Parameter(HelpMessage=@"
禁用 PyPI 镜像源, 使用 PyPI 官方源下载 Python 软件包
"@)][switch]$DisablePyPIMirror,

    [Parameter(HelpMessage=@"
禁用 Qwen TTS WebUI Installer 更新检查
"@)][switch]$DisableUpdate,

    [Parameter(HelpMessage=@"
禁用 Qwen TTS WebUI Installer 使用 uv 安装 Python 软件包, 使用 Pip 安装 Python 软件包
"@)][switch]$DisableUV,

    [Parameter(HelpMessage=@"
禁用 Qwen TTS WebUI Installer 自动设置代理服务器
"@)][switch]$DisableProxy,

    [Parameter(HelpMessage=@"
使用自定义的代理服务器地址, 例如代理服务器地址为 http://127.0.0.1:10809, 则使用 -UseCustomProxy `"http://127.0.0.1:10809`" 设置代理服务器地址
"@)][string]$UseCustomProxy,

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
        DisableUV = $script:DisableUV
        DisableProxy = $script:DisableProxy
        UseCustomProxy = $script:UseCustomProxy
        DisablePyPIMirror = $script:DisablePyPIMirror
        BuildMode = $script:BuildMode
        DisableUpdate = $script:DisableUpdate
        NoPause = $script:NoPause
    }
    (Import-Module (Join-Path $PSScriptRoot "modules.psm1") -Function "Join-NormalizedPath", "Initialize-EnvPath", "Write-Log", "Set-CorePrefix", "Get-Version", "Set-PyPIMirror", "Update-Installer", "Set-uv", "Set-Proxy", "Update-SDWebUiAllInOne", "Get-HelpMessage", "Test-PythonAndGit" -PassThru -Force -ErrorAction Stop).Invoke({
        param ($cfg)
        $script:OriginalScriptPath = $cfg.OriginalScriptPath
        $script:LaunchCommandLine = $cfg.LaunchCommandLine
        $script:Help = $cfg.Help
        $script:CorePrefix = $cfg.CorePrefix
        $script:DisableUV = $cfg.DisableUV
        $script:DisableProxy = $cfg.DisableProxy
        $script:UseCustomProxy = $cfg.UseCustomProxy
        $script:DisablePyPIMirror = $cfg.DisablePyPIMirror
        $script:BuildMode = $cfg.BuildMode
        $script:DisableUpdate = $cfg.DisableUpdate
        $script:NoPause = $cfg.NoPause
    }, $config)
}
catch {
    Write-Error "导入 Installer 模块发生错误: $_"
    Write-Host "这可能是 Installer 文件出现了损坏, 请运行 " -ForegroundColor White -NoNewline
    Write-Host "launch_qwen_tts_webui_installer.ps1" -ForegroundColor Yellow -NoNewline
    Write-Host " 脚本修复该问题" -ForegroundColor White
    if (!($script:BuildMode)) { if (!($script:NoPause)) { Read-Host | Out-Null } }
    exit 1
}


# 获取启动 SD WebUI All In One 内核的启动参数
function Get-LaunchCoreArgs {
    $launch_params = New-Object System.Collections.ArrayList
    Set-PyPIMirror $launch_params
    Set-uv $launch_params
    if ($script:BuildWithTorch) {
        $launch_params.Add("--index") | Out-Null
        $launch_params.Add($BuildWithTorch) | Out-Null
    }
    if ($script:BuildWithTorchReinstall) {
        $launch_params.Add("--force-reinstall") | Out-Null
    }
    if (!($script:BuildMode)) {
        $launch_params.Add("--interactive") | Out-Null
    }
    return $launch_params
}


function Main {
    Get-HelpMessage
    Get-Version
    Set-CorePrefix
    Initialize-EnvPath
    Test-PythonAndGit
    Set-Proxy
    Update-Installer
    Update-SDWebUiAllInOne

    $launch_args = Get-LaunchCoreArgs
    & python -m sd_webui_all_in_one qwen-tts-webui reinstall-pytorch $launch_args

    Write-Log "退出 PyTorch 重装脚本"
    if (!($script:BuildMode)) { if (!($script:NoPause)) { Read-Host | Out-Null } }
}

###################

Main